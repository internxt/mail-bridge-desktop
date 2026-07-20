#!/usr/bin/env python3
"""
mail_bridge.py

USAGE:
  sudo python3 mail_bridge.py
"""
import hashlib
import argparse
import asyncio
import base64
import contextlib
import logging
import re
import ssl
import time
import requests
import email as email_pkg
import json

from datetime import datetime, timezone
from email.message import EmailMessage as MimeMessage
from email.utils import formatdate, make_msgid, getaddresses, parseaddr
from requests.adapters import HTTPAdapter


IDLE_POLL_INTERVAL = 5  # seconds between remote checks while idling

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
log = logging.getLogger("bridge-mail-server")

SPECIAL_USE = {
    "SENT": "\\Sent",
    "DRAFTS": "\\Drafts",
    "TRASH": "\\Trash",
    "JUNK": "\\Junk",
    "ARCHIVE": "\\Archive",
}
REMOTE_TYPE_BY_MAILBOX = {
    "INBOX": "inbox",
    "SENT": "sent",
    "TRASH": "trash",
    "JUNK": "spam",
}
REMOVAL_MAILBOXES = ("Trash", "Junk")

from internxt_api import (
    login,
    list_emails,
    send_email,
    decrypt_mail,
    encrypt_outgoing_email,
    get_thread,
    parse_encryption_block,
    is_encrypted_email_body,
    download_attachment,
    delete_email,
    update_email,
    decrypt_attachment,
)
def _is_html(body: str) -> bool:
    return bool(re.search(r"<\s*(html|body|div|p|br|table)\b", body, re.IGNORECASE))

async def remote_email_to_message(summary: dict, next_uid: int, store: "MailboxStore") -> dict:
    from_list = summary.get("from") or []
    to_list = summary.get("to") or []
    from_name = from_list[0].get("name") or from_list[0].get("email", "") if from_list else ""
    from_addr = from_list[0].get("email", "") if from_list else ""
    to_name = to_list[0].get("name") or to_list[0].get("email", "") if to_list else ""
    to_addr = to_list[0].get("email", "") if to_list else ""

    encryption = summary.get("encryption")
    if encryption and not isinstance(encryption, dict):
        subject = f"[encrypted] {summary.get('subject', '')}"
        body = "[DECRYPTION FAILED: legacy format, not supported]"
    elif encryption:
        wrapped_keys = encryption.get("wrappedKeys")
        encrypted_preview = encryption.get("encryptedPreview")
        result = await decrypt_mail(store, wrapped_keys, encrypted_preview, encrypted_preview, encrypted_preview)
        if result.get("ok"):
            attachments_session_key = result.get("attachmentsSessionKey")
            subject = summary.get("subject", "")
            body = result.get("body", "")
        else:
            subject = f"[encrypted] {summary.get('subject', '')}"
            body = f"[DECRYPTION FAILED: {result.get('error')}]"
    else:
        subject = summary.get("subject", "")
        body = summary.get("preview", "")

    msg = MimeMessage()
    msg["From"] = f"{from_name} <{from_addr}>"
    msg["To"] = f"{to_name} <{to_addr}>"
    msg["Subject"] = subject
    received = summary.get("receivedAt")
    try:
        dt = datetime.fromisoformat(received.replace("Z", "+00:00")) if received else datetime.now(timezone.utc)
    except ValueError:
        dt = datetime.now(timezone.utc)
    msg["Date"] = formatdate(dt.timestamp(), localtime=False)
    msg["Message-ID"] = f"<{summary['id']}@internxt.mail>"
    if _is_html(body):
        msg.set_content(body, subtype="html")
    else:
        msg.set_content(body)


    raw = msg.as_bytes()
    return {
        "uid": next_uid,
        "remote_id": summary["id"],
        "raw": raw,
        "size": len(raw),
        "flags": set() if not summary.get("isRead") else {"\\Seen"},
        "subject": subject,
        "from_name": from_name,
        "from_addr": from_addr,
        "to_name": to_name,
        "to_addr": to_addr,
        "date_header": msg["Date"],
        "message_id": msg["Message-ID"],
        "internaldate": dt,
        "body_loaded": False,
    }

def rebuild_msg_raw(msg: dict, body: str, attachments: list | None = None):
    m = MimeMessage()
    m["From"] = f'{msg["from_name"]} <{msg["from_addr"]}>'
    m["To"] = f'{msg["to_name"]} <{msg["to_addr"]}>'
    m["Subject"] = msg["subject"]
    m["Date"] = msg["date_header"]
    m["Message-ID"] = msg["message_id"]
    if _is_html(body):
        m.set_content(body, subtype="html")
    else:
        m.set_content(body)
    for att_name, att_type, att_bytes in (attachments or []):
        maintype, _, subtype = (att_type or "application/octet-stream").partition("/")
        m.add_attachment(att_bytes, maintype=maintype or "application", subtype=subtype or "octet-stream", filename=att_name)
    raw = m.as_bytes()
    msg["raw"] = raw
    msg["size"] = len(raw)


def item_needs_full_body(item: str) -> bool:
    key = item.upper()
    if key in ("RFC822", "RFC822.HEADER", "RFC822.TEXT", "BODYSTRUCTURE", "BODY"):
        return True
    return bool(re.match(r"^BODY(\.PEEK)?\[", item, re.IGNORECASE))


# ---------------------------------------------------------------------------
# Mailbox
# ---------------------------------------------------------------------------

class MailboxStore:
    def __init__(self):
        self.mailboxes: dict = {}
        self.dirty: set = set()
        self.revisions: dict = {}
        self.last_synced_at: dict = {}
        self._poller_task: asyncio.Task | None = None
        self._poller_lock = asyncio.Lock()
        self._active_idlers: set = set() 
        self._fetch_locks: dict = {}
        self.session = requests.Session()
        self.mnemonic: str | None = None
        self._cached_private_key: str | None = None
        self.token: str | None = None
        self.email: str | None = None
        self.get_or_create("INBOX")
        self.get_or_create("Sent")
        adapter = HTTPAdapter(pool_connections=20, pool_maxsize=20)
        self.session.mount("https://", adapter)
        self.get_or_create("Trash")
        self.get_or_create("Drafts")
        self.get_or_create("Junk")

    def _lock_for(self, name: str) -> asyncio.Lock:
        key = name.upper()
        if key not in self._fetch_locks:
            self._fetch_locks[key] = asyncio.Lock()
        return self._fetch_locks[key]

    def invalidate_auth(self, reason: str = ""):
        log.warning("Invalidating cached token%s", f": {reason}" if reason else "")
        self.token = None
        
    def mark_dirty(self, name: str):
        self.dirty.add(name.upper())

    def register_idler(self, display_name: str):
        self._active_idlers.add(display_name.upper())

    def unregister_idler(self, display_name: str):
        self._active_idlers.discard(display_name.upper())

    def bump_revision(self, name: str):
        key = name.upper()
        self.revisions[key] = self.revisions.get(key, 0) + 1

    def revision(self, name: str) -> int:
        return self.revisions.get(name.upper(), 0)

    async def ensure_poller_running(self):
        async with self._poller_lock:
            if self._poller_task is None or self._poller_task.done():
                self._poller_task = asyncio.create_task(self._poll_loop())

    async def fetch_and_merge(self, display_name: str, remote_type: str,  wait: bool = True) -> bool:
        lock = self._lock_for(display_name)
        if not wait and lock.locked():
            return False
        async with lock:
            if not self.token:
                return False
            _, mb = self.find(display_name)
            if mb is None:
                return False
            loop = asyncio.get_running_loop()
            try:
                summaries = await loop.run_in_executor(None, list_emails, self.session, self.token, remote_type)
            except Exception as e:
                if is_auth_error(e):
                    self.invalidate_auth("Token is invalid")
                else:
                    log.exception("Failed to fetch remote emails for %s", display_name)
                return False
            elsewhere_ids = set()
            for special_name in REMOVAL_MAILBOXES:
                if display_name.upper() == special_name.upper():
                    continue
                _, special_mb = self.find(special_name)
                if special_mb is not None:
                    elsewhere_ids |= special_mb._remote_ids
            added_any = False
            for s in summaries:
                remote_id = s.get("id")
                if mb.has_remote_id(remote_id) or remote_id in elsewhere_ids:
                    continue
                msg_dict = await remote_email_to_message(s, mb.next_uid, self)
                uid = mb.add_remote_message(msg_dict, True)
                if uid is not None:
                    added_any = True
            if added_any:
                self.bump_revision(display_name)
            self.dirty.discard(display_name.upper())
            return added_any

    async def sync_removed_and_reconcile(self):
        if not self.token:
            return
        newly_elsewhere = set()
        for special_name in REMOVAL_MAILBOXES:
            remote_type = REMOTE_TYPE_BY_MAILBOX[special_name.upper()]
            _, special_mb = self.find(special_name)
            if special_mb is None:
                continue
            before_ids = set(special_mb._remote_ids)
            await self.fetch_and_merge(special_name, remote_type)
            newly_elsewhere |= (special_mb._remote_ids - before_ids)
        if not newly_elsewhere:
            return
        for name in self.names():
            if name.upper() in (m.upper() for m in REMOVAL_MAILBOXES):
                continue
            _, mb = self.find(name)
            if mb is None:
                continue
            removed = False
            for msg in list(mb.messages):
                remote_id = msg.get("remote_id")
                if remote_id in newly_elsewhere:
                    seq = mb.seq_of(msg["uid"])
                    mb.messages.remove(msg)
                    mb._remote_ids.discard(remote_id)
                    if seq is not None:
                        mb.pending_expunges.append(seq)
                    removed = True
            if removed:
                self.bump_revision(name)

    async def ensure_full_body(self, msg: dict):
        if msg.get("body_loaded"):
            return
        remote_id = msg.get("remote_id")
        if remote_id is None:
            msg["body_loaded"] = True
            return
        lock = self._lock_for(f"body:{remote_id}")
        async with lock:
            if msg.get("body_loaded"):
                return
            if not self.token:
                return
            loop = asyncio.get_running_loop()
            try:
                thread = await loop.run_in_executor(None, get_thread, self.session, self.token, remote_id)
            except Exception as e:
                if is_auth_error(e):
                    self.invalidate_auth("Token is invalid")
                else:
                    log.exception("Failed to fetch full body for %s", remote_id)
                return
            full = next((m for m in thread if m.get("id") == remote_id), None)
            if full is None:
                log.error("Message %s not found in its own thread response", remote_id)
                msg["body_loaded"] = True
                return

            text_body = full.get("textBody") or ""
            attachments_session_key = None
            if is_encrypted_email_body(text_body):
                try:
                    encryption = parse_encryption_block(text_body)
                except Exception:
                    log.exception("Failed to parse encryption block for %s", remote_id)
                    body = "[DECRYPTION FAILED: could not parse encryption block]"
                else:
                    wrapped_keys = encryption.get("wrappedKeys")
                    encrypted_text = encryption.get("encryptedText")
                    version = encryption.get("version")
                    encrypted_preview = encryption.get("encryptedPreview")
                    encrypted_session_key = encryption.get("encryptedAttachmentsSessionKey")
                    result = await decrypt_mail(self, wrapped_keys, encrypted_text, encrypted_preview, encrypted_session_key, version)
                    if result.get("ok"):
                        attachments_session_key = result.get("attachmentsSessionKey")
                        try:
                            body = json.loads(result.get("body", "")).get("body", "")
                        except (json.JSONDecodeError, AttributeError):
                            body = result.get("body", "")
                    else:
                        body = f"[DECRYPTION FAILED: {result.get('error')}]"
            else:
                body = text_body or full.get("htmlBody") or ""

            attachments_data = []
            for att in full.get("attachments") or []:
                blob_id = att.get("blobId")
                if not blob_id:
                    continue
                try:
                    data = await loop.run_in_executor(
                        None, download_attachment, self.session, self.token, remote_id, blob_id, att.get("name"), att.get("type"),
                    )
                except Exception:
                    log.exception("Failed to download attachment %s for %s", blob_id, remote_id)
                    continue
                if is_encrypted_email_body(text_body):
                    if not attachments_session_key:
                        log.warning("No attachments session key for %s; skipping %s", remote_id, blob_id)
                        continue
                    try:
                        data = await decrypt_attachment(attachments_session_key, data)
                    except Exception:
                        log.exception("Failed to decrypt attachment %s for %s", blob_id, remote_id)
                        continue
                attachments_data.append((att.get("name", "attachment"), att.get("type", "application/octet-stream"), data))
            rebuild_msg_raw(msg, body, attachments_data)
            msg["body_loaded"] = True

    async def _poll_loop(self):
        log.info("Starting unified remote-mail poller")
        try:
            while True:
                await asyncio.sleep(IDLE_POLL_INTERVAL)
                if not self.token:
                    continue
                active = list(self._active_idlers)
                for display_name in active:
                    remote_type = REMOTE_TYPE_BY_MAILBOX.get(display_name)
                    if remote_type:
                        await self.fetch_and_merge(display_name, remote_type)
                await self.sync_removed_and_reconcile()
        except asyncio.CancelledError:
            log.info("Poller cancelled")
            raise


    def get_or_create(self, name: str) -> "Mailbox":
        _, existing = self.find(name)
        if existing is not None:
            return existing
        mb = Mailbox()
        self.mailboxes[name] = mb
        return mb

    def find(self, name: str):
        key = name.upper()
        for existing_name, mb in self.mailboxes.items():
            if existing_name.upper() == key:
                return existing_name, mb
        return None, None

    def names(self):
        return list(self.mailboxes.keys())

    def special_use(self, name: str):
        return SPECIAL_USE.get(name.upper())


def imap_list_match(pattern: str, name: str) -> bool:
    if pattern in ("*", "%"):
        return True
    return pattern.upper() == name.upper()


def stable_uidvalidity(email: str) -> int:
    h = hashlib.sha256(email.encode()).digest()
    return int.from_bytes(h[:4], 'big') & 0x7FFFFFFF 

def is_auth_error(exc: Exception) -> bool:
    resp = getattr(exc, "response", None)
    status = getattr(resp, "status_code", None)
    if status in (401, 403):
        return True
    msg = str(exc).lower()
    return any(k in msg for k in ("401", "unauthorized", "expired", "invalid token", "jwt"))

class Mailbox:
    def __init__(self):
        self.uidvalidity = 0
        self.next_uid = 1
        self.messages: list = []
        self.recent_uids: set = set()
        self._remote_ids: set = set()
        self.pending_expunges: list = []

    def has_remote_id(self, remote_id: str) -> bool:
        return remote_id in self._remote_ids

    def add_remote_message(self, msg_dict: dict, mark_recent: bool = False):
        remote_id = msg_dict.get("remote_id")
        if remote_id is not None and remote_id in self._remote_ids:
            return None  # already present, skip
        msg_dict["uid"] = self.next_uid
        self.next_uid += 1
        self.messages.append(msg_dict)
        if remote_id is not None:
            self._remote_ids.add(remote_id)
        if mark_recent:
            self.recent_uids.add(msg_dict["uid"])
        return msg_dict["uid"]

    def add_raw_message(self, raw: bytes, flags=None, mark_recent: bool = True) -> int:
        parsed = email_pkg.message_from_bytes(raw)
        from_name, from_addr = parseaddr(parsed.get("From", ""))
        to_name, to_addr = parseaddr(parsed.get("To", ""))
        data = {
            "uid": self.next_uid,
            "raw": raw,
            "size": len(raw),
            "flags": set(flags or []),
            "subject": parsed.get("Subject", ""),
            "from_name": from_name or from_addr,
            "from_addr": from_addr,
            "to_name": to_name or to_addr,
            "to_addr": to_addr,
            "date_header": parsed.get("Date", formatdate(localtime=False)),
            "message_id": parsed.get("Message-ID", make_msgid(domain="fakemail.local")),
            "internaldate": datetime.now(timezone.utc),
        }
        self.next_uid += 1
        self.messages.append(data)
        if mark_recent:
            self.recent_uids.add(data["uid"])
        return data["uid"]

    @property
    def exists(self) -> int:
        return len(self.messages)

    @property
    def recent(self) -> int:
        return len(self.recent_uids)

    def clear_recent(self):
        self.recent_uids.clear()

    def seq_of(self, uid: int):
        for i, m in enumerate(self.messages):
            if m["uid"] == uid:
                return i + 1
        return None

    def by_seq(self, seq: int):
        if 1 <= seq <= len(self.messages):
            return self.messages[seq - 1]
        return None

    def by_uid(self, uid: int):
        for m in self.messages:
            if m["uid"] == uid:
                return m
        return None


# ---------------------------------------------------------------------------
# IMAP protocol helpers
# ---------------------------------------------------------------------------

class Literal:
    __slots__ = ("data",)

    def __init__(self, data: bytes):
        self.data = data


def tokenize(s: str) -> list:
    tokens = []
    buf = []
    depth = 0
    in_quotes = False
    token_active = False
    i, n = 0, len(s)
    while i < n:
        c = s[i]
        if in_quotes:
            token_active = True
            if c == "\\" and i + 1 < n:
                buf.append(s[i + 1])
                i += 2
                continue
            if c == '"':
                in_quotes = False
                i += 1
                continue
            buf.append(c)
            i += 1
            continue
        if c == '"':
            in_quotes = True
            token_active = True
            i += 1
            continue
        if c in "([":
            depth += 1
            token_active = True
            buf.append(c)
            i += 1
            continue
        if c in ")]":
            depth = max(0, depth - 1)
            buf.append(c)
            i += 1
            continue
        if c == " " and depth == 0:
            if token_active:
                tokens.append("".join(buf))
                buf = []
                token_active = False
            i += 1
            continue
        token_active = True
        buf.append(c)
        i += 1
    if token_active:
        tokens.append("".join(buf))
    return tokens


def parse_seq_set(spec: str, max_value: int) -> list:
    """Parses a sequence set like '1:3,5,7:*' bounded by max_value (used for
    both message sequence numbers and UIDs)."""
    if max_value <= 0:
        return []
    result = set()
    for part in spec.split(","):
        if ":" in part:
            lo, hi = part.split(":", 1)
            lo = 1 if lo == "*" else int(lo)
            hi = max_value if hi == "*" else int(hi)
            if lo > hi:
                lo, hi = hi, lo
            result.update(range(lo, hi + 1))
        else:
            val = max_value if part == "*" else int(part)
            result.add(val)
    return sorted(v for v in result if 1 <= v <= max_value)


def imap_quote(s) -> str:
    if s is None:
        return "NIL"
    escaped = str(s).replace("\\", "\\\\").replace('"', '\\"')
    return f'"{escaped}"'


def imap_address_list(name: str, addr: str) -> str:
    mailbox, _, host = addr.partition("@")
    return f'(({imap_quote(name)} NIL {imap_quote(mailbox)} {imap_quote(host)}))'


def build_envelope(msg: dict) -> str:
    date_str = imap_quote(msg["date_header"])
    subject = imap_quote(msg["subject"])
    from_ = imap_address_list(msg["from_name"], msg["from_addr"])
    to_ = imap_address_list(msg["to_name"], msg["to_addr"])
    message_id = imap_quote(msg["message_id"])
    return f"({date_str} {subject} {from_} {from_} {from_} {to_} NIL NIL NIL {message_id})"


def build_bodystructure(msg: dict) -> str:
    size = msg["size"]
    lines = msg["raw"].count(b"\n")
    return f'("TEXT" "PLAIN" ("CHARSET" "UTF-8") NIL NIL "7BIT" {size} {lines})'


def expand_fetch_macro(items_spec: str) -> list:
    spec = items_spec.strip()
    if spec.startswith("(") and spec.endswith(")"):
        spec = spec[1:-1]
    upper = spec.upper()
    if upper == "ALL":
        return ["FLAGS", "INTERNALDATE", "RFC822.SIZE", "ENVELOPE"]
    if upper == "FULL":
        return ["FLAGS", "INTERNALDATE", "RFC822.SIZE", "ENVELOPE", "BODY"]
    if upper == "FAST":
        return ["FLAGS", "INTERNALDATE", "RFC822.SIZE"]
    return tokenize(spec)


def fetch_item_response(msg: dict, item: str):
    key = item.upper()

    if key == "FLAGS":
        return "FLAGS", f'({" ".join(sorted(msg["flags"]))})'
    if key == "UID":
        return "UID", str(msg["uid"])
    if key == "RFC822.SIZE":
        return "RFC822.SIZE", str(msg["size"])
    if key == "INTERNALDATE":
        return "INTERNALDATE", imap_quote(msg["internaldate"].strftime("%d-%b-%Y %H:%M:%S +0000"))
    if key == "ENVELOPE":
        return "ENVELOPE", build_envelope(msg)
    if key in ("BODYSTRUCTURE", "BODY"):
        return "BODYSTRUCTURE", build_bodystructure(msg)
    if key == "RFC822":
        msg["flags"].add("\\Seen")
        return "RFC822", Literal(msg["raw"])
    if key == "RFC822.HEADER":
        header, _, _ = msg["raw"].partition(b"\r\n\r\n")
        return "RFC822.HEADER", Literal(header + b"\r\n\r\n")
    if key == "RFC822.TEXT":
        _, _, body = msg["raw"].partition(b"\r\n\r\n")
        msg["flags"].add("\\Seen")
        return "RFC822.TEXT", Literal(body)

    m = re.match(r"^BODY(\.PEEK)?\[(.*?)\]$", item, re.IGNORECASE)
    if m:
        peek = bool(m.group(1))
        section = m.group(2).upper()
        header_bytes, _, body_bytes = msg["raw"].partition(b"\r\n\r\n")
        if section == "":
            data = msg["raw"]
        elif section == "HEADER":
            data = header_bytes + b"\r\n\r\n"
        elif section == "TEXT":
            data = body_bytes
        elif section.startswith("HEADER.FIELDS"):
            wanted = []
            if "(" in section:
                wanted = re.findall(r"[\w.-]+", section.split("(", 1)[1])
            wanted_lower = {w.lower() for w in wanted}
            keep_lines = []
            keep = False
            for line in header_bytes.decode("utf-8", errors="replace").split("\r\n"):
                if line and line[0] in " \t":
                    if keep:
                        keep_lines.append(line)
                    continue
                field = line.split(":", 1)[0].lower()
                keep = field in wanted_lower
                if keep:
                    keep_lines.append(line)
            data = ("\r\n".join(keep_lines) + "\r\n\r\n").encode("utf-8")
        else:
            data = msg["raw"]
        if not peek:
            msg["flags"].add("\\Seen")
        response_label = f"BODY[{m.group(2)}]"
        return response_label, Literal(data)


async def read_command(reader: asyncio.StreamReader, writer: asyncio.StreamWriter):
    raw = b""
    while True:
        line = await reader.readline()
        if not line:
            return None
        raw += line
        check = line.rstrip(b"\r\n")
        m = re.search(rb"\{(\d+)(\+?)\}$", check)
        if not m:
            break
        size = int(m.group(1))
        non_sync = m.group(2) == b"+"
        if not non_sync:
            writer.write(b"+ OK\r\n")
            await writer.drain()
        raw += await reader.readexactly(size)
    return raw.decode("utf-8", errors="replace").rstrip("\r\n")

        
# ---------------------------------------------------------------------------
# IMAP session (one per connected client)
# ---------------------------------------------------------------------------

class IMAPSession:
    SYNC_TTL_SECONDS = 15  # how long a mailbox's remote fetch stays "fresh"

    def __init__(self, reader, writer, store: MailboxStore, ssl_ctx: ssl.SSLContext):
        self.reader = reader
        self.writer = writer
        self.store = store
        self.state = "NONAUTH"
        self.selected_name = None
        self.peer = writer.get_extra_info("peername")
        self.ssl_ctx = ssl_ctx

    async def _sync_mailbox_from_remote(self, display_name: str, mb: "Mailbox", force: bool = False):
        key = display_name.upper()
        remote_type = REMOTE_TYPE_BY_MAILBOX.get(key)
        if remote_type is None:
            return
        last = self.store.last_synced_at.get(key)
        is_dirty = key in self.store.dirty
        needs_refresh = force or last is None or is_dirty or (time.monotonic() - last) > self.SYNC_TTL_SECONDS
        if not needs_refresh:
            return
        if last is None or is_dirty:
            await self.store.fetch_and_merge(display_name, remote_type)
            self.store.last_synced_at[key] = time.monotonic()
        else:
            self.store.last_synced_at[key] = time.monotonic()
            asyncio.create_task(self.store.fetch_and_merge(display_name, remote_type, wait=False)) 

    @property
    def mailbox(self):
        if self.selected_name is None:
            return None
        _, mb = self.store.find(self.selected_name)
        return mb

    async def require_valid_token(self, tag: str) -> bool:
        if self.store.token:
            return True
        self.state = "NONAUTH"
        await self.send(f"{tag} NO [AUTHENTICATIONFAILED] Session expired, please reconnect")
        return False

    async def send(self, line: str):
        log.info("IMAP response: %s", line)
        self.writer.write((line + "\r\n").encode("utf-8"))
        await self.writer.drain()

    async def send_fetch_line(self, seq: int, parts: list):
        self.writer.write(f"* {seq} FETCH (".encode())
        for idx, (name, value) in enumerate(parts):
            if idx > 0:
                self.writer.write(b" ")
            self.writer.write(f"{name} ".encode())
            if isinstance(value, Literal):
                self.writer.write(f"{{{len(value.data)}}}\r\n".encode())
                self.writer.write(value.data)
            else:
                self.writer.write(value.encode())
        self.writer.write(b")\r\n")
        await self.writer.drain()
        log.info("IMAP FETCH sent for seq=%d, %d bytes total in literals",
            seq, sum(len(v.data) for _, v in parts if isinstance(v, Literal)))

    async def run(self):
        log.info("Connection from %s", self.peer)
        await self.send("* OK [CAPABILITY IMAP4rev1 SPECIAL-USE IDLE] Basic IMAP server ready")
        try:
            while True:
                raw = await read_command(self.reader, self.writer)
                if raw is None:
                    break
                if not raw:
                    continue
                await self.dispatch(raw)
                if self.writer.is_closing():
                    break
        except (ConnectionResetError, BrokenPipeError):
            pass
        except Exception as e:
            log.exception("IMAP session error: %s", e)
        finally:
            log.info("Connection closed %s", self.peer)
            if not self.writer.is_closing():
                self.writer.close()

    async def dispatch(self, raw: str):
        parts = raw.split(None, 2)
        if len(parts) < 2:
            return
        tag = parts[0]
        command = parts[1].upper().strip()
        rest = parts[2] if len(parts) > 2 else ""
        handler = getattr(self, f"cmd_{command.lower().strip()}", None)
        try:
            if handler is None:
                await self.send(f"{tag} BAD Unknown command")
                return
            await handler(tag, rest)
        except Exception:
            log.exception("Error handling command: %s", raw)
            with contextlib.suppress(ConnectionResetError, BrokenPipeError):
                await self.send(f"{tag} BAD Internal error")

    # ---- commands ----
    IDLE_CHECK_INTERVAL = 1
    IDLE_MAX_DURATION = 25 * 60

    async def cmd_capability(self, tag, rest):
        await self.send("* CAPABILITY IMAP4rev1 SPECIAL-USE IDLE")
        await self.send(f"{tag} OK CAPABILITY completed")

    async def cmd_idle(self, tag, rest):
        if self.state != "SELECTED":
            await self.send(f"{tag} BAD IDLE requires a selected mailbox")
            return

        mb = self.mailbox
        display_name = self.selected_name
        await self.store.ensure_poller_running()
        self.store.register_idler(display_name)
        log.info("IDLE started for %s", display_name)

        done_event = asyncio.Event()

        async def watch_for_done():
            try:
                while True:
                    line = await self.reader.readline()
                    if not line:
                        break
                    if line.strip().upper() == b"DONE":
                        break
            except (ConnectionResetError, BrokenPipeError):
                pass
            finally:
                done_event.set()

        watcher = asyncio.create_task(watch_for_done())
        start_revision = self.store.revision(display_name)
        start = time.monotonic()

        try:
            while not done_event.is_set():
                if time.monotonic() - start > self.IDLE_MAX_DURATION:
                    break
                try:
                    await asyncio.wait_for(done_event.wait(), timeout=self.IDLE_CHECK_INTERVAL)
                    break 
                except asyncio.TimeoutError:
                    pass 

                current_revision = self.store.revision(display_name)
                if current_revision != start_revision:
                    while mb.pending_expunges:
                        seq = mb.pending_expunges.pop(0)
                        await self.send(f"* {seq} EXPUNGE")
                    await self.send(f"* {mb.exists} EXISTS")
                    await self.send(f"* {mb.recent} RECENT")
                    start_revision = current_revision
        finally:
            self.store.unregister_idler(display_name)
            if not watcher.done():
                watcher.cancel()
                with contextlib.suppress(asyncio.CancelledError):
                    await watcher
        try:
            await self.send(f"{tag} OK IDLE completed")
        except (ConnectionResetError, BrokenPipeError):
            log.info("Client disconnected during IDLE for %s (tag=%s)", display_name, tag)

    async def cmd_noop(self, tag, rest):
        await self.send(f"{tag} OK NOOP completed")

    async def cmd_id(self, tag, rest):
        await self.send("* ID NIL")
        await self.send(f"{tag} OK ID completed")

    async def cmd_sort(self, tag, rest):
        await self.send("* SORT")
        await self.send(f"{tag} OK SORT completed")


    async def cmd_thread(self, tag, rest):
        await self.send("* THREAD")
        await self.send(f"{tag} OK THREAD completed")

    async def cmd_namespace(self, tag, rest):
        await self.send(f"{tag} BAD Unknown command")

    async def cmd_starttls(self, tag, rest):
        await self.send(f"{tag} OK Begin TLS negotiation")
        loop = asyncio.get_running_loop()
        new_transport = await loop.start_tls(
            self.writer.transport, self.writer.transport.get_protocol(),
            self.ssl_ctx, server_side=True,
        )
        self.writer._transport = new_transport

    async def cmd_logout(self, tag, rest):
        try:
            self.writer.write(b"* BYE logging out\r\n")
            self.writer.write(f"{tag} OK LOGOUT completed\r\n".encode())
            await self.writer.drain()
        except (ConnectionResetError, BrokenPipeError):
            pass
        finally:
            self.writer.close()

    async def cmd_enable(self, tag, rest):
        await self.send("* ENABLED")
        await self.send(f"{tag} OK ENABLE completed")

    async def cmd_login(self, tag, rest):
        tokens = tokenize(rest)
        if len(tokens) < 2:
            await self.send(f"{tag} BAD LOGIN needs a username and password")
            return
        email, password = tokens[0], tokens[1]

        if self.store.token and self.store.email == email:
            self.state = "AUTH"
            await self.store.ensure_poller_running()
            await self.send(f"{tag} OK LOGIN completed")
            return

        loop = asyncio.get_running_loop()
        try:
            result = await loop.run_in_executor(None, login,  self.store.session, email, password)
        except Exception as e:
            log.exception("Real login failed for %s", email)
            await self.send(f"{tag} NO [AUTHENTICATIONFAILED] Login failed: {e}")
            return

        token = result.get("newToken")
        if not token:
            await self.send(f"{tag} NO [AUTHENTICATIONFAILED] No token returned")
            return

        self.state = "AUTH"
        self.store.token = token
        self.store.mnemonic = result.get("_decrypted_mnemonic")
        for name in self.store.names():
            _, mb = self.store.find(name)
            mb.uidvalidity = stable_uidvalidity(email)
        self.store.email = email
        await self.store.ensure_poller_running()
        await self.send(f"{tag} OK LOGIN completed")


    async def cmd_list(self, tag, rest):
        if self.state == "NONAUTH":
            await self.send(f"{tag} NO Please login first")
            return
        tokens = tokenize(rest)
        if tokens and tokens[0].startswith("("):
            tokens = tokens[1:]
        mailbox_pattern = tokens[1] if len(tokens) > 1 else ""
        if mailbox_pattern == "":
            await self.send('* LIST (\\Noselect) "/" ""')
        else:
            for name in self.store.names():
                if imap_list_match(mailbox_pattern, name):
                    attrs = ["\\HasNoChildren"]
                    su = self.store.special_use(name)
                    if su:
                        attrs.append(su)
                    await self.send(f'* LIST ({" ".join(attrs)}) "/" {name}')
        await self.send(f"{tag} OK LIST completed")

    async def cmd_lsub(self, tag, rest):
        await self.cmd_list(tag, rest)

    async def cmd_select(self, tag, rest, readonly=False):
        if self.state == "NONAUTH":
            await self.send(f"{tag} NO Please login first")
            return
        tokens = tokenize(rest)
        requested = tokens[0].strip('"') if tokens else ""
        display_name, mb = self.store.find(requested)
        if mb is None:
            log.warning("SELECT requested unknown mailbox: %r (known: %s)", requested, self.store.names())
            await self.send(f"{tag} NO Mailbox does not exist")
            return

        await self._sync_mailbox_from_remote(display_name, mb)
        if not await self.require_valid_token(tag):
            return

        await self.send(f"* {mb.exists} EXISTS")
        await self.send(f"* {mb.recent} RECENT")
        first_unseen = next((i + 1 for i, m in enumerate(mb.messages) if "\\Seen" not in m["flags"]), None)
        if first_unseen:
            await self.send(f"* OK [UNSEEN {first_unseen}] Message {first_unseen} is first unseen")
        await self.send(f"* OK [UIDVALIDITY {mb.uidvalidity}] UIDs valid")
        await self.send(f"* OK [UIDNEXT {mb.next_uid}] Predicted next UID")
        await self.send("* FLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)")
        await self.send("* OK [PERMANENTFLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft)] Limited")
        self.state = "SELECTED"
        self.selected_name = display_name
        mb.clear_recent()
        mode = "READ-ONLY" if readonly else "READ-WRITE"
        await self.send(f"{tag} OK [{mode}] {'EXAMINE' if readonly else 'SELECT'} completed")


    async def cmd_examine(self, tag, rest):
        await self.cmd_select(tag, rest, readonly=True)

    async def cmd_status(self, tag, rest):
        tokens = tokenize(rest)
        name = tokens[0].strip('"') if tokens else "INBOX"
        display_name, mb = self.store.find(name)
        if mb is None:
            log.warning("SELECT requested unknown mailbox: %r (known: %s)", name, self.store.names())
            await self.send(f"{tag} NO Mailbox does not exist")
            return
        await self._sync_mailbox_from_remote(display_name, mb)
        if not await self.require_valid_token(tag):
            return
        unseen = sum(1 for m in mb.messages if "\\Seen" not in m["flags"])
        await self.send(
            f"* STATUS {display_name} (MESSAGES {mb.exists} RECENT {mb.recent} "
            f"UIDNEXT {mb.next_uid} UIDVALIDITY {mb.uidvalidity} UNSEEN {unseen})"
        )
        await self.send(f"{tag} OK STATUS completed")

    async def cmd_create(self, tag, rest):
        tokens = tokenize(rest)
        if not tokens:
            await self.send(f"{tag} BAD CREATE needs a mailbox name")
            return
        self.store.get_or_create(tokens[0].strip('"'))
        await self.send(f"{tag} OK CREATE completed")
    
    async def _expunge(self, tag, rest, send_responses: bool):
        if self.state != "SELECTED":
            await self.send(f"{tag} BAD No mailbox selected")
            return
        mb = self.mailbox
        to_remove = [m for m in mb.messages if "\\Deleted" in m["flags"]]
        loop = asyncio.get_running_loop()
        for msg in to_remove:
            remote_id = msg.get("remote_id")
            if remote_id and self.store.token:
                try:
                    await loop.run_in_executor(None, delete_email, self.store.session, self.store.token, remote_id)
                except Exception as e:
                    if is_auth_error(e):
                        self.store.invalidate_auth("Token is invalid")
                    else:
                        log.exception("Failed to delete remote email %s", remote_id)
                    continue
            seq = mb.seq_of(msg["uid"])
            mb.messages.remove(msg)
            if remote_id:
                mb._remote_ids.discard(remote_id)
            if send_responses and seq is not None:
                await self.send(f"* {seq} EXPUNGE")
        self.store.bump_revision(self.selected_name)
        await self.send(f"{tag} OK {'EXPUNGE' if send_responses else 'CLOSE'} completed")

    async def cmd_close(self, tag, rest):
        await self._expunge(tag, rest, send_responses=False)
        self.state = "AUTH"

    async def cmd_expunge(self, tag, rest):
        await self._expunge(tag, rest, send_responses=True)

    async def cmd_fetch(self, tag, rest, use_uid=False):
        if self.state != "SELECTED":
            await self.send(f"{tag} BAD No mailbox selected")
            return
        tokens = tokenize(rest)
        if len(tokens) < 2:
            await self.send(f"{tag} BAD FETCH needs a sequence set and data items")
            return
        seq_spec, items_spec = tokens[0], " ".join(tokens[1:])
        mb = self.mailbox

        if use_uid:
            max_val = mb.next_uid - 1
            uids = [u for u in parse_seq_set(seq_spec, max_val) if mb.by_uid(u)]
        else:
            max_val = mb.exists
            uids = [mb.by_seq(s)["uid"] for s in parse_seq_set(seq_spec, max_val) if mb.by_seq(s)]

        items = expand_fetch_macro(items_spec)
        needs_body = any(item_needs_full_body(it) for it in items)
        if needs_body:
            msgs = [mb.by_uid(uid) for uid in uids]
            sem = asyncio.Semaphore(8)
            async def _bounded(m):
                async with sem:
                    await self.store.ensure_full_body(m)
            await asyncio.gather(*(_bounded(m) for m in msgs))
        for uid in uids:
            msg = mb.by_uid(uid)
            seq = mb.seq_of(uid)
            response_parts = []
            if use_uid and not any(it.upper() == "UID" for it in items):
                response_parts.append(("UID", str(uid)))
            for item in items:
                name, value = fetch_item_response(msg, item)
                if name is not None:
                    response_parts.append((name, value))
            await self.send_fetch_line(seq, response_parts)
        await self.send(f"{tag} OK {'UID ' if use_uid else ''}FETCH completed")

    async def cmd_store(self, tag, rest, use_uid=False):
        if self.state != "SELECTED":
            await self.send(f"{tag} BAD No mailbox selected")
            return
        tokens = tokenize(rest)
        if len(tokens) < 3:
            await self.send(f"{tag} BAD STORE needs a sequence set, action, and flags")
            return
        seq_spec, action = tokens[0], tokens[1].upper()
        flags = tokenize(" ".join(tokens[2:]).strip("()"))
        mb = self.mailbox
        if use_uid:
            max_val = mb.next_uid - 1
            uids = [u for u in parse_seq_set(seq_spec, max_val) if mb.by_uid(u)]
        else:
            max_val = mb.exists
            uids = [mb.by_seq(s)["uid"] for s in parse_seq_set(seq_spec, max_val) if mb.by_seq(s)]

        for uid in uids:
            msg = mb.by_uid(uid)
            was_seen = "\\Seen" in msg["flags"]
            if action.startswith("+"):
                msg["flags"].update(flags)
            elif action.startswith("-"):
                msg["flags"].difference_update(flags)
            else:
                msg["flags"] = set(flags)
            now_seen = "\\Seen" in msg["flags"]
            remote_id = msg.get("remote_id")
            if now_seen != was_seen and remote_id and self.store.token:
                try:
                    await asyncio.to_thread(
                        update_email, self.store.session, self.store.token,
                        remote_id, is_read=now_seen)
                except Exception as e:
                    if is_auth_error(e):
                        self.store.invalidate_auth("Token is invalid")
                    else:
                        log.exception("Failed to sync isRead for %s", remote_id)

            if "SILENT" not in action:
                seq = mb.seq_of(uid)
                await self.send_fetch_line(seq, [("FLAGS", f'({" ".join(sorted(msg["flags"]))})')])
        await self.send(f"{tag} OK {'UID ' if use_uid else ''}STORE completed")

    async def cmd_search(self, tag, rest, use_uid=False):
        if self.state != "SELECTED":
            await self.send(f"{tag} BAD No mailbox selected")
            return
        criteria = rest.strip().upper()
        mb = self.mailbox
        matches = []
        for i, m in enumerate(mb.messages):
            ident = m["uid"] if use_uid else i + 1
            if criteria in ("", "ALL"):
                matches.append(ident)
            elif criteria == "UNSEEN" and "\\Seen" not in m["flags"]:
                matches.append(ident)
            elif criteria == "SEEN" and "\\Seen" in m["flags"]:
                matches.append(ident)
        await self.send(f"* SEARCH {' '.join(str(x) for x in matches)}")
        await self.send(f"{tag} OK {'UID ' if use_uid else ''}SEARCH completed")

    async def cmd_uid(self, tag, rest):
        tokens = rest.split(" ", 1)
        sub = tokens[0].upper() if tokens else ""
        sub_rest = tokens[1] if len(tokens) > 1 else ""
        if sub == "FETCH":
            await self.cmd_fetch(tag, sub_rest, use_uid=True)
        elif sub == "STORE":
            await self.cmd_store(tag, sub_rest, use_uid=True)
        elif sub == "SEARCH":
            await self.cmd_search(tag, sub_rest, use_uid=True)
        elif sub == "COPY":
            await self.cmd_uid_copy(tag, sub_rest)
        else:
            await self.send(f"{tag} BAD Unsupported UID subcommand")

    async def cmd_uid_copy(self, tag, rest):
        if self.state != "SELECTED":
            await self.send(f"{tag} BAD No mailbox selected")
            return
        tokens = tokenize(rest)
        if len(tokens) < 2:
            await self.send(f"{tag} BAD UID COPY needs a sequence set and destination mailbox")
            return
        seq_spec, dest_name = tokens[0], tokens[1].strip('"')
        mb = self.mailbox
        max_val = mb.next_uid - 1
        uids = [u for u in parse_seq_set(seq_spec, max_val) if mb.by_uid(u)]

        dest_key = dest_name.upper()
        if dest_key not in ("TRASH", "JUNK"):
            await self.send(f"{tag} OK UID COPY completed (no-op)")
            return

        loop = asyncio.get_running_loop()
        for uid in uids:
            msg = mb.by_uid(uid)
            remote_id = msg.get("remote_id") if msg else None
            if not remote_id or not self.store.token:
                continue
            try:
                if dest_key == "TRASH":
                    await loop.run_in_executor(None, delete_email, self.store.session, self.store.token, remote_id)
                else:
                    await loop.run_in_executor(None, update_email, self.store.session, self.store.token, remote_id, "spam")  
            except Exception as e:
                if is_auth_error(e):
                    self.store.invalidate_auth("Token is invalid")
                else:
                    log.exception("Failed to move %s to %s via UID COPY", remote_id, dest_key.lower())
                continue
            seq = mb.seq_of(uid)
            mb.messages.remove(msg)
            mb._remote_ids.discard(remote_id)
            if seq is not None:
                mb.pending_expunges.append(seq)
        self.store.bump_revision(self.selected_name)
        await self.send(f"{tag} OK UID COPY completed")

    async def cmd_append(self, tag, rest):
        if self.state == "NONAUTH":
            await self.send(f"{tag} NO Please login first")
            return

        m = re.search(r"\{(\d+)\+?\}\r\n", rest)
        if not m:
            await self.send(f"{tag} BAD APPEND requires a message literal")
            return
        size = int(m.group(1))
        payload = rest[m.end():m.end() + size]
        header_part = rest[:m.start()]
        tokens = tokenize(header_part)
        mailbox_name = tokens[0].strip('"') if tokens else "INBOX"

        if mailbox_name.upper() == "SENT":
            mb = self.store.get_or_create(mailbox_name)
            log.info("Skipping local APPEND into Sent (already synced from remote)")
            await self.send(f"{tag} OK [APPENDUID {mb.uidvalidity} {mb.next_uid}] APPEND completed")
            return

        mb = self.store.get_or_create(mailbox_name)
        raw_bytes = payload.encode("utf-8", errors="replace")
        uid = mb.add_raw_message(raw_bytes, flags={"\\Seen"})
        log.info("APPEND stored message uid=%s (%d bytes) into %r", uid, len(raw_bytes), mailbox_name)
        await self.send(f"{tag} OK [APPENDUID {mb.uidvalidity} {uid}] APPEND completed")

def should_encrypt_for(req_body: dict) -> bool:
    recipients = req_body.get("to", []) + req_body.get("cc", [])
    if not recipients:
        return False
    internal_domains = ("@inxt.com", "@inxt.me")
    return all(r["email"].lower().endswith(internal_domains) for r in recipients)



# --------------------------
# Basic SMTP server
# --------------------------
def make_smtp_handler(ssl_ctx, store: MailboxStore):
    async def handle_smtp_client(reader, writer):
        peer = writer.get_extra_info("peername")
        log.info("SMTP connection from %s", peer)
        loop = asyncio.get_running_loop()
        authenticated_token = None

        async def send(line):
            log.info("SMTP response: %s", line)
            writer.write((line + "\r\n").encode())
            await writer.drain()

        await send("220 localhost Basic SMTP ready")

        try:
            while True:
                line = await reader.readline()
                if not line:
                    break
                cmd = line.decode(errors="ignore").strip()
                upper = cmd.upper()

                if upper.startswith(("EHLO", "HELO")):
                    await send("250-localhost")
                    await send("250 AUTH PLAIN LOGIN")

                elif upper.startswith("AUTH PLAIN"):
                    parts = cmd.split(" ", 2)
                    if len(parts) == 3:
                        b64 = parts[2]
                    else:
                        await send("334 ")
                        resp_line = await reader.readline()
                        b64 = resp_line.decode().strip()
                    try:
                        _, username, password = base64.b64decode(b64).decode("utf-8", errors="replace").split("\0")
                        result = await loop.run_in_executor(None, login, store.session, username, password)
                        authenticated_token = result.get("newToken")
                    except Exception:
                        log.exception("SMTP AUTH PLAIN failed")
                        authenticated_token = None
                    await send("235 Authentication successful" if authenticated_token else "535 Authentication failed")

                elif upper.startswith("AUTH LOGIN"):
                    await send("334 VXNlcm5hbWU6")
                    user_line = await reader.readline()
                    await send("334 UGFzc3dvcmQ6")
                    pass_line = await reader.readline()
                    try:
                        username = base64.b64decode(user_line.decode().strip()).decode("utf-8", errors="replace")
                        password = base64.b64decode(pass_line.decode().strip()).decode("utf-8", errors="replace")
                        result = await loop.run_in_executor(None, login, store.session, username, password)
                        authenticated_token = result.get("newToken")
                    except Exception:
                        log.exception("SMTP AUTH LOGIN failed")
                        authenticated_token = None
                    await send("235 Authentication successful" if authenticated_token else "535 Authentication failed")

                elif upper.startswith("MAIL FROM"):
                    await send("250 OK")
                elif upper.startswith("RCPT TO"):
                    await send("250 OK")

                elif upper.startswith("DATA"):
                    await send("354 End data with <CR><LF>.<CR><LF>")
                    lines = []
                    while True:
                        dline = await reader.readline()
                        if not dline or dline in (b".\r\n", b".\n"):
                            break
                        if dline.startswith(b".."):
                            dline = dline[1:]
                        lines.append(dline)
                    raw = b"".join(lines)

                    if not authenticated_token:
                        log.error("DATA received without successful AUTH; rejecting")
                        await send("530 Authentication required")
                    else:
                        try:
                            req_body = build_send_email_request(raw)
                            if should_encrypt_for(req_body):
                                req_body = await encrypt_outgoing_email(store, req_body)
                            await loop.run_in_executor(None, send_email, authenticated_token, req_body)
                        except Exception as e:
                            if is_auth_error(e):
                                store.invalidate_auth("Token is not valid")
                                await send("530 Authentication required")
                            else:
                                log.exception("Failed to send email via real API")
                                await send("554 Transaction failed: could not send message")
                        else:
                            try:
                                store.mark_dirty("Sent")
                            except Exception:
                                log.exception("mark_dirty failed after successful send (non-fatal)")
                            await send("250 OK: message sent")

                elif upper.startswith("QUIT"):
                    await send("221 Bye")
                    break
                elif upper == "STARTTLS":
                    await send("220 Ready to start TLS")
                    new_transport = await loop.start_tls(
                        writer.transport, writer.transport.get_protocol(), ssl_ctx, server_side=True,
                    )
                    writer._transport = new_transport
                else:
                    await send("250 OK")
        except Exception:
            pass
        finally:
            writer.close()
            log.info("SMTP connection closed %s", peer)

    return handle_smtp_client


def _addr_list(header_val):
    if not header_val:
        return []
    out = []
    for name, addr in getaddresses([header_val]):
        entry = {"email": addr}
        if name:
            entry["name"] = name
        out.append(entry)
    return out

def build_send_email_request(raw: bytes) -> dict:
    parsed = email_pkg.message_from_bytes(raw)
    text_body, html_body = None, None
    if parsed.is_multipart():
        for part in parsed.walk():
            ctype = part.get_content_type()
            if ctype == "text/plain" and text_body is None:
                text_body = (part.get_payload(decode=True) or b"").decode(part.get_content_charset() or "utf-8", errors="replace")
            elif ctype == "text/html" and html_body is None:
                html_body = (part.get_payload(decode=True) or b"").decode(part.get_content_charset() or "utf-8", errors="replace")
    else:
        payload = (parsed.get_payload(decode=True) or b"").decode(parsed.get_content_charset() or "utf-8", errors="replace")
        if parsed.get_content_type() == "text/html":
            html_body = payload
        else:
            text_body = payload

    body = {
        "to": _addr_list(parsed.get("To")),
        "subject": parsed.get("Subject", ""),
    }
    cc = _addr_list(parsed.get("Cc"))
    if cc:
        body["cc"] = cc
    if text_body is not None:
        body["textBody"] = text_body
    if html_body is not None:
        body["htmlBody"] = html_body
    return body

# ---------------------------------------------------------------------------
# Bootstrap
# ---------------------------------------------------------------------------

def build_ssl_context(cert_path="cert.pem", key_path="key.pem"):
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    ctx.load_cert_chain(certfile=cert_path, keyfile=key_path)
    return ctx


async def main_async(args):
    store = MailboxStore()
    ssl_ctx = build_ssl_context()
    async def handle_imap(reader, writer):
        await IMAPSession(reader, writer, store, ssl_ctx).run()
    smtp_handler = make_smtp_handler(ssl_ctx, store)
    imap_tls = await asyncio.start_server(handle_imap, args.host, 993, ssl=ssl_ctx)
    smtp_tls = await asyncio.start_server(smtp_handler, args.host, 465, ssl=ssl_ctx)
    log.info("IMAPS (TLS) on %s:993", args.host)
    log.info("SMTPS (TLS) on %s:465", args.host)
    async with imap_tls, smtp_tls:
        await asyncio.gather(imap_tls.serve_forever(), smtp_tls.serve_forever())

def main():
    parser = argparse.ArgumentParser(
        description="Basic local IMAP/SMTP server for testing Apple Mail integration"
    )
    parser.add_argument("--host", default="127.0.0.1")
    args = parser.parse_args()
    try:
        asyncio.run(main_async(args))
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()