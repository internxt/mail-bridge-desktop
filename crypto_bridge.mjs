// crypto_bridge.mjs
import { encryptEmailHybridForMultipleRecipients, genSymmetricKey, decryptSymmetrically, encryptSymmetrically, decryptEmailHybrid, openEncryptionKeystore } from 'internxt-crypto'; 

class NoWrappedKeyError extends Error {
  constructor(email) {
    super(`No wrapped key found for recipient: ${email}`);
    this.name = 'NoWrappedKeyError';
    this.code = 'NO_WRAPPED_KEY_FOR_RECIPIENT';
  }
}

function b64ToBytes(b64) {
  return new Uint8Array(Buffer.from(b64, 'base64'));
}
function bytesToB64(bytes) {
  return Buffer.from(bytes).toString('base64');
}

async function readStdin() {
  const chunks = [];
  for await (const chunk of process.stdin) chunks.push(chunk);
  return Buffer.concat(chunks).toString('utf-8');
}

async function decryptForMe(wrappedKeys, encText, encPreview, encAttachmentsSessionKey, secretKeyBytes, myEmail) {
  const normalized = myEmail.toLowerCase();
  const mine = wrappedKeys.find(w => w.encryptedForEmail?.toLowerCase() === normalized);
  if (!mine) throw new NoWrappedKeyError(myEmail);
  return decryptEmailHybrid({ encText, encPreview, encAttachmentsSessionKey }, mine, secretKeyBytes);

}

async function main() {
  let input;
  try {
    input = JSON.parse(await readStdin());
  } catch (err) {
    process.stdout.write(JSON.stringify({ ok: false, error: `bad input JSON: ${err.message}` }));
    process.exit(1);
  }

  try {
    if (input.action === 'open_keystore') {
      const keys = await openEncryptionKeystore(input.encryptedKeystore, input.mnemonic);
      process.stdout.write(JSON.stringify({
        ok: true,
        keys: {
          publicKey: bytesToB64(keys.publicKey),
          secretKey: bytesToB64(keys.secretKey),
        },
      }));
    } else if (input.action === 'decrypt') {

      const secretKeyBytes = b64ToBytes(input.secretKey);

      const {text, preview, attachmentsSessionKey} = await decryptForMe(input.wrappedKeys, input.encryptedText, input.encryptedPreview, input.encryptedAttachmentsSessionKey, secretKeyBytes, input.myEmail);

      process.stdout.write(JSON.stringify({
        ok: true,
        body: text,
        preview,
        attachmentsSessionKey: attachmentsSessionKey?.length ? bytesToB64(attachmentsSessionKey) : null,
      }));
       
    } else if (input.action === 'encrypt') {
      if (!input.recipients || input.recipients.length === 0) {
        throw new Error('no recipients provided');
      } 

      const recipients = input.recipients.map(r => ({
        email: r.email,
        publicHybridKey: b64ToBytes(r.publicHybridKey),
      }));
      const email = {
        text: input.email.text,
        preview: input.preview,
        attachmentsSessionKey: input.attachmentsSessionKey ? b64ToBytes(input.attachmentsSessionKey) : new Uint8Array(),
      };

      const { encryptedKeys, encEmail } = await
        encryptEmailHybridForMultipleRecipients(email, recipients);

      const result = {
        version: 'v3',
        encryptedText: encEmail.encText,
        wrappedKeys: encryptedKeys,
        encryptedPreview: encEmail.encPreview,
        encryptedAttachmentsSessionKey: encEmail.encAttachmentsSessionKey,
      };
      process.stdout.write(JSON.stringify({ ok: true, result }));
    } else if (input.action === 'generate_session_key') {
      const key = genSymmetricKey();
      process.stdout.write(JSON.stringify({ ok: true, sessionKey: bytesToB64(key) }));
     } else if (input.action === 'decrypt_attachment') {
        const plaintext = await decryptSymmetrically(
        b64ToBytes(input.sessionKey),
        b64ToBytes(input.data),
      );
      process.stdout.write(JSON.stringify({ ok: true, data: bytesToB64(plaintext) }));
     } else if (input.action === 'encrypt_attachment') {
       const ciphertext = await encryptSymmetrically(
         b64ToBytes(input.sessionKey),
         b64ToBytes(input.data),
       );
       process.stdout.write(JSON.stringify({ ok: true, data: bytesToB64(ciphertext) }));
      }
     else {
      process.stdout.write(JSON.stringify({ ok: false, error: `unknown action: ${input.action}` }));
      process.exit(1);
    }
  } catch (err) {
    process.stdout.write(JSON.stringify({
      ok: false,
      error: err instanceof Error ? err.message : String(err),
      code: err?.code ?? null,
    })); process.exit(1);
  }
}

main();