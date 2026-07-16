#!/usr/bin/env python3
"""
internxt_api.py

"""

import hashlib
import json
import logging
import os
import sys
import asyncio
 
import requests
 
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes

MAIL_API_URL = "https://gateway.internxt.com/mail"
AUTH_API_URL = "https://gateway.internxt.com/drive"
CRYPTO_BRIDGE_PATH = os.path.join(os.path.dirname(__file__), "crypto_bridge.mjs")

async def call_crypto_bridge(payload: dict) -> dict:
    proc = await asyncio.create_subprocess_exec(
        "node", CRYPTO_BRIDGE_PATH,
        stdin=asyncio.subprocess.PIPE,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    stdin_data = json.dumps(payload).encode("utf-8")
    stdout_data, stderr_data = await proc.communicate(stdin_data)
    if proc.returncode != 0 and not stdout_data:
        raise RuntimeError(f"crypto_bridge.mjs failed: {stderr_data.decode('utf-8', errors='replace')}")
    text = stdout_data.decode("utf-8")
    try:
        result, _ = json.JSONDecoder().raw_decode(text)
    except json.JSONDecodeError as e:
        raise RuntimeError(f"crypto_bridge.mjs produced unparseable output: {text!r}")     
    if not result.get("ok"):
        raise RuntimeError(f"crypto_bridge error: {result.get('error')}")
    return result

def lookup_public_keys(session, token, addresses: list) -> list:
    resp = session.post(
        f"{MAIL_API_URL}/email/keys/lookup",
        json={"addresses": addresses},
        headers=auth_headers(token),
        timeout=15,
    )
    if not resp.ok:
        log.error("lookup_public_keys failed (%s): %s", resp.status_code, resp.text)
        resp.raise_for_status()
    return resp.json().get("recipients", [])

def download_keystore(session: requests.Session, token: str, user_email: str, keystore_type: str = "Encryption") -> dict:
    resp = session.get(
        f"{MAIL_API_URL}/users/me/mail-account/keys",
        params={"userEmail": user_email, "keystoreType": keystore_type},
        headers=auth_headers(token),
        timeout=15,
    )
    if not resp.ok:
        log.error("download_keystore failed (%s): %s", resp.status_code, resp.text)
        resp.raise_for_status()
    return resp.json()


async def get_my_decrypted_private_key(store: "MailboxStore") -> str | None:
    if not store.token or not store.mnemonic or not store.email:
        return None
    if store._cached_private_key is not None:
        return store._cached_private_key
    loop = asyncio.get_running_loop()
    try:
        encrypted_keystore = await loop.run_in_executor(
            None, download_keystore, store.session, store.token, store.email, "Encryption"
        )
        mapped_keystore = {
            "userEmail": encrypted_keystore["address"],
            "type": "Encryption",
            "publicKey": encrypted_keystore["publicKey"],
            "privateKeyEncrypted": encrypted_keystore["encryptionPrivateKey"],
        }
        result = await call_crypto_bridge({
            "action": "open_keystore",
            "encryptedKeystore": mapped_keystore,
            "mnemonic": store.mnemonic,
        })
        store._cached_private_key = result["keys"]["secretKey"]
        return store._cached_private_key
    except Exception:
        log.exception("Failed to fetch/open encryption keystore")
        return None

async def decrypt_mail(store: "MailboxStore", encryption: dict) -> dict:
    private_key_b64 = await get_my_decrypted_private_key(store)
    if not private_key_b64 or not store.email:
        return {"ok": False, "error": f"no keys available for {store.email}"}
    wrapped_keys = encryption.get("wrappedKeys")
    encrypted_text = encryption.get("encryptedPreview")
    if not wrapped_keys or not encrypted_text:
        return {"ok": False, "error": f"legacy format, wrappedKeys or encryptedPreview are missing"}
    try:
        return await call_crypto_bridge({
            "action": "decrypt",
            "encryptedText": encrypted_text,
            "wrappedKeys": wrapped_keys,
            "secretKey": private_key_b64,
            "myEmail": 'tamara-test@inxt.me', #TODO: change to store.email once account creation works (!!!!!!)
        })
    except Exception:
        log.exception("Failed to decrypt mail preview")
        return {"ok": False, "error": "decrypt failed"}

async def generate_attachments_session_key() -> str:
    result = await call_crypto_bridge({"action": "generate_session_key"})
    return result["sessionKey"]

async def encrypt_outgoing_email(store: "MailboxStore", req_body: dict) -> dict:
    addresses = [r["email"] for r in req_body.get("to", [])] + [r["email"] for r in req_body.get("cc", [])]
    loop = asyncio.get_running_loop()
    looked_up = await loop.run_in_executor(
        None, lookup_public_keys, store.session, store.token, addresses
    )
    recipients = [
        {"email": r["address"], "publicHybridKey": r["publicKey"]}
        for r in looked_up
    ]
    attachments_session_key = ""
    if req_body.get("attachments"):
    	attachments_session_key = await generate_attachments_session_key()
   
    result = await call_crypto_bridge({
        "action": "encrypt",
        "email": {"text": req_body.get("textBody", "")},
        "previewText": req_body.get("textBody", "")[:256],
        "attachmentsSessionKey": attachments_session_key,
        "recipients": recipients,
    })
    new_body = dict(req_body)
    new_body.pop("textBody", None)
    new_body.pop("htmlBody", None)
    block = result["result"]
    new_body["encryptedText"] = block["encryptedText"]
    new_body["wrappedKeys"] = block["wrappedKeys"]
    new_body["encryptedPreview"] = block["encryptedPreview"]
    new_body["previewWrappedKeys"] = block["previewWrappedKeys"]
    
    new_body["encryption"] = result["result"]
    return new_body

# ---------------------------------------------------------------------------
# passToHash: PBKDF2-HMAC-SHA1(password, salt, 10000 iters, 32-byte output)
# ---------------------------------------------------------------------------

def pass_to_hash(password: str, salt_hex: str | None = None) -> dict:
    salt = bytes.fromhex(salt_hex) if salt_hex else os.urandom(16)
    derived = hashlib.pbkdf2_hmac("sha1", password.encode("utf-8"), salt, 10000, dklen=32)
    return {"salt": salt.hex(), "hash": derived.hex()}


# ---------------------------------------------------------------------------
# CryptoJS-compatible AES-256-CBC with OpenSSL "Salted__" key derivation
# ---------------------------------------------------------------------------

def _get_key_and_iv(secret: str, salt: bytes) -> tuple:
    password = secret.encode("utf-8") + salt
    md5_hashes = []
    digest = password
    for _ in range(3):
        h = hashlib.md5(digest).digest()
        md5_hashes.append(h)
        digest = h + password
    key = md5_hashes[0] + md5_hashes[1]  # 32 bytes
    iv = md5_hashes[2]                   # 16 bytes
    return key, iv


def _pkcs7_pad(data: bytes, block_size: int = 16) -> bytes:
    pad_len = block_size - (len(data) % block_size)
    return data + bytes([pad_len]) * pad_len


def _pkcs7_unpad(data: bytes) -> bytes:
    pad_len = data[-1]
    return data[:-pad_len]


def encrypt_text_with_key(text: str, secret: str) -> str:
    salt = os.urandom(8)
    key, iv = _get_key_and_iv(secret, salt)

    cipher = Cipher(algorithms.AES(key), modes.CBC(iv))
    encryptor = cipher.encryptor()
    padded = _pkcs7_pad(text.encode("utf-8"))
    ciphertext = encryptor.update(padded) + encryptor.finalize()

    return (b"Salted__" + salt + ciphertext).hex()


def decrypt_text_with_key(encrypted_hex: str, secret: str) -> str:
    raw = bytes.fromhex(encrypted_hex)
    salt = raw[8:16]
    ciphertext = raw[16:]
    key, iv = _get_key_and_iv(secret, salt)

    cipher = Cipher(algorithms.AES(key), modes.CBC(iv))
    decryptor = cipher.decryptor()
    padded = decryptor.update(ciphertext) + decryptor.finalize()

    return _pkcs7_unpad(padded).decode("utf-8")


def encrypt_password_hash(password: str, encrypted_salt: str) -> str:
    app_crypto_secret = "6KYQBP847D4ATSFA"
    salt_hex = decrypt_text_with_key(encrypted_salt, app_crypto_secret)
    hashed = pass_to_hash(password, salt_hex)
    return encrypt_text_with_key(hashed["hash"], app_crypto_secret)


log = logging.getLogger("bridge-mail.api")

def auth_headers(token: str) -> dict:
    return {**basic_headers(), "Authorization": f"Bearer {token}"}

def auth_headers_mail(token: str) -> dict:
    return {**basic_headers_mail(), "Authorization": f"Bearer {token}"}

def basic_headers() -> dict:
    return {
        "Content-Type": "application/json; charset=utf-8",
        "Accept": "application/json, text/plain, */*",
        "internxt-client": "drive-web",
        "internxt-version": "v1.0.810",
    }

def basic_headers_mail() -> dict:
    return {
        "Content-Type": "application/json; charset=utf-8",
        "Accept": "application/json, text/plain, */*",
        "internxt-client": "mail-web",
        "internxt-version": "v1.0.810",
    }

def headers(token: str) -> dict:
    return {
        "Content-Type": "application/json; charset=utf-8",
        "Accept": "application/json, text/plain, */*",
        "internxt-client": "drive-web",
        "internxt-version": "v1.0.810",
        "token": token,
    } 

def security_details(session: requests.Session, email: str) -> dict:
    resp = session.post(
        f"{AUTH_API_URL}/auth/login",
        json={"email": email},
        headers=basic_headers(),
        timeout=15,
    )
    if not resp.ok:
        print(f"security_details failed ({resp.status_code}): {resp.text}", file=sys.stderr)
        resp.raise_for_status()
    data = resp.json()
    return {
        "encrypted_salt": data["sKey"],
    }


def login(session: requests.Session, email: str, password: str) -> dict:
    details = security_details(session, email)

    encrypted_password_hash = encrypt_password_hash(password, details["encrypted_salt"])

    body = {
        "email": email,
        "password": encrypted_password_hash,
        "tfa": "",
        "privateKey": None,
        "publicKey": None,
        "revocateKey": None,
        "keys": None,
    }

    resp = session.post(
        f"{AUTH_API_URL}/auth/login/access",
        json=body,
        headers=basic_headers(),
        timeout=15,
    )
    if not resp.ok:
        print(f"Login failed ({resp.status_code}): {resp.text}", file=sys.stderr)
        resp.raise_for_status()

    data = resp.json()

    encrypted_mnemonic = data.get("user", {}).get("mnemonic")
    if encrypted_mnemonic:
        try:
            data["_decrypted_mnemonic"] = decrypt_text_with_key(encrypted_mnemonic, password)
        except Exception:
            log.exception("Failed to decrypt mnemonic from login response")
            data["_decrypted_mnemonic"] = None
    else:
        data["_decrypted_mnemonic"] = None

    return data

def list_emails(session: requests.Session, token: str, mailbox: str, limit: int = 50, position: int = 0) -> list:
    resp = session.get(
        f"{MAIL_API_URL}/email",
        params={"mailbox": mailbox, "limit": limit, "position": position},
        headers=auth_headers(token),
        timeout=15,
    )
    if not resp.ok:
        print(f"list_emails({mailbox}) failed ({resp.status_code}): {resp.text}", file=sys.stderr)
        resp.raise_for_status()
    return resp.json().get("emails", [])



def send_email(token: str, body: dict) -> dict:
    resp = requests.post(f"{MAIL_API_URL}/email/send", json=body, headers=auth_headers(token), timeout=15)
    if not resp.ok:
        log.error("send_email failed (%s): %s", resp.status_code, resp.text)
        resp.raise_for_status()
    return resp.json()

