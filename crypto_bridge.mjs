// crypto_bridge.mjs
import { encryptEmailHybrid, encryptEmailHybridForMultipleRecipients, genSymmetricKey, decryptEmailHybrid, openEncryptionKeystore } from 'internxt-crypto'; 


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

export const ENCRYPTED_EMAIL_PREFIX = 'INTERNXT-ENCRYPTED-EMAIL-v1';

async function decryptForMe(wrappedKeys, ciphertextB64, secretKeyBytes, myEmail) {
  const normalized = myEmail.toLowerCase();
  const mine = wrappedKeys.find(w => w.encryptedForEmail?.toLowerCase() === normalized);
  if (!mine) throw new Error('No wrapped key found for this recipient');
  const encEmail = { encryptedKey: mine, encEmail: { encText: ciphertextB64 } };
  return decryptEmailHybrid(encEmail, secretKeyBytes);
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

      const {text} = await decryptForMe(input.wrappedKeys, input.encryptedText, secretKeyBytes, input.myEmail);

      process.stdout.write(JSON.stringify({ ok: true, body: text }));
       
    } else if (input.action === 'encrypt') {
      if (!input.recipients || input.recipients.length === 0) {
        throw new Error('no recipients provided');
      } 

      const recipients = input.recipients.map(r => ({
        email: r.email,
        publicHybridKey: b64ToBytes(r.publicHybridKey),
      }));
      const bodyPayload = {
        body: input.email.text,
        attachmentsSessionKey: input.attachmentsSessionKey ?? '',
      };

      const [encryptedBodies, encryptedPreviews] = await Promise.all([
        encryptEmailHybridForMultipleRecipients({ text: JSON.stringify(bodyPayload) }, recipients),
        encryptEmailHybridForMultipleRecipients({ text: input.previewText ?? ' ' }, recipients),
      ]);

      const result = {
        version: 'v2',
        encryptedText: encryptedBodies[0].encEmail.encText,
        wrappedKeys: encryptedBodies.map(e => e.encryptedKey),
        encryptedPreview: encryptedPreviews[0].encEmail.encText,
        previewWrappedKeys: encryptedPreviews.map(e => e.encryptedKey),
      };
      process.stdout.write(JSON.stringify({ ok: true, result }));
    } else if (input.action === 'generate_session_key') {
      const key = genSymmetricKey();
      process.stdout.write(JSON.stringify({ ok: true, sessionKey: bytesToB64(key) }));
     }
     else {
      process.stdout.write(JSON.stringify({ ok: false, error: `unknown action: ${input.action}` }));
      process.exit(1);
    }
  } catch (err) {
    process.stdout.write(JSON.stringify({ ok: false, error: err instanceof Error ? err.message : String(err) }));
    process.exit(1);
  }
}

main();