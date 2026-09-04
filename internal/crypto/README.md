# internal/crypto

This is a port of the [JS crypto library](https://github.com/internxt/crypto) the web client uses
gets subtly wrong makes real mail unreadable.

## Why any of this exists

Mail bodies are encrypted on the sender's machine and stay encrypted on the
server. Only the recipients can read them. The subject travels in cleartext, so
the backend can index it.

An email is encrypted **once**, not once per recipient. A random _session key_
seals the body, and that session key is then wrapped separately for each
recipient. Everyone unwraps the same session key with their own private key.

## The chain

Four steps, from what arrives over the wire to readable text:

```
text body  ──▶  envelope  ──▶  session key  ──▶  plaintext
            1              2                 3
```

**1. Parse the envelope** — `ParseEnvelope` (envelope.go)

An encrypted email's text body is not text. It is a marker line followed by
base64-encoded JSON:

```
INTERNXT-ENCRYPTED-EMAIL-v1
eyJ2ZXJzaW9uIjoidjMiLCJlbmNyeXB0ZWRUZXh0Ijoi...
```

`IsEncryptedBody` reports whether a body looks like this. The JSON holds three
ciphertexts (body, preview, attachments key) and one wrapped key per recipient,
each labeled with the address it was wrapped for.

**2. Recover the session key** — `DecryptKeysHybrid` (hybrid.go)

Find the wrapped key labeled for this account, then open it. Two sub-steps,
covered below.

**3. Decrypt the ciphertexts** — `DecryptEmail` (email.go)

The session key decrypts the body, the preview and the attachments session key,
all three with AES-GCM.

`DecryptEnvelope` runs all of it end to end and is what callers normally use.

## The cryptography we use

### AES-GCM — `DecryptSymmetrically`

The ordinary case: one key, encrypt and decrypt with it. GCM also authenticates,
so a modified ciphertext fails to decrypt instead of returning garbage.

One quirk to know about, because it looks like a bug: **the IV goes at the end
of the buffer**, not the beginning as is customary. The layout is
`ciphertext || tag || iv`. That is what the JS library does, so that is what we
have to read. There is a test that fails if someone "fixes" it.

### AES-KW — `UnwrapKey`

A cipher specialised in encrypting _keys_ rather than messages. Ordinary AES-GCM
would need a random IV and add a tag, so wrapping 32 bytes would cost far more
than 32 bytes. AES-KW has neither: 32 bytes in, 40 bytes out.

Its integrity check is the trick worth understanding. Instead of a separate tag,
unwrapping must reproduce a fixed constant, `A6A6A6A6A6A6A6A6`. With the right
key it always does; with the wrong one the odds are 1 in 2⁶⁴. That is why a bad
key gives `ErrKeyUnwrap` rather than a plausible-looking wrong key.

It is written out by hand here (about 40 lines) because Go's standard library
does not have it, and neither does `golang.org/x/crypto`. See _Verification_
below for why that is safe.

### The hybrid KEM — `DecapsulateHybrid`

A **KEM** (key encapsulation mechanism) is how a sender agrees on a secret with
a recipient they have never spoken to, knowing only their public key. The sender
encapsulates: out comes a shared secret plus a ciphertext. The recipient
decapsulates that ciphertext with their private key and arrives at the same
shared secret. That shared secret is what wraps the session key.

**Hybrid** means two KEMs at once, and the reason is quantum computers:

- **X25519** — classical elliptic-curve exchange. Well understood, decades of
  scrutiny, and breakable by a large enough quantum computer.
- **ML-KEM-768** — post-quantum (formerly Kyber). Believed safe against quantum
  attack, but much younger and less battle-tested.

Running both and combining the results means an attacker has to break _both_.
X25519 covers the risk that ML-KEM turns out to be flawed; ML-KEM covers the
risk of a quantum computer. This matters for email specifically: a message
captured today can be stored and decrypted years from now.

Three things happen inside:

_Seed expansion._ The private key stored is a single 32-byte seed, not two keys.
SHAKE256 stretches it into 96 bytes: the first 64 are the ML-KEM seed, the last
32 the X25519 private key. Both component keys are re-derived on every call.

_Ciphertext split._ The 1120-byte hybrid ciphertext is 1088 bytes of ML-KEM
followed by 32 bytes of X25519.

_The combiner._ The two shared secrets become one:

```
SHA3-256( ss_mlkem ‖ ss_x25519 ‖ ct_x25519 ‖ pk_x25519 ‖ "\.//^\" )
```

Feeding in the ciphertext and the public key, not just the two secrets, binds
the result to this specific exchange. The trailing label is domain separation:
it keeps this hash from colliding with the same inputs hashed for another
purpose. Every byte of it matters — a different label yields a different secret
and an unwrap that fails with no useful clue.

## Encrypting

The other direction, for sending mail. Each function below is the mirror of
one above, run in the opposite order — seal the ciphertexts first, then wrap
the key per recipient — and built from the same primitives:

```
Email  ──▶  session key  ──▶  envelope  ──▶  text body
        1                 2               3
```

**1. Generate a session key and seal the email** — `EncryptEmail` (email.go)

A fresh random 32-byte key, used once, encrypts the body, preview and
attachments session key with `EncryptSymmetrically` (the mirror of
`DecryptSymmetrically`: same AES-GCM, same ciphertext‖tag‖iv layout, new
random IV every call — reusing an IV under the same key would break GCM's
confidentiality guarantee entirely).

**2. Wrap the session key for every recipient** — `EncryptKeysHybrid` (hybrid.go)

`EncapsulateHybrid(recipientPublicKey)` agrees on a shared secret with a
recipient knowing only their public key, then `WrapKey` (the mirror of
`UnwrapKey`) seals the session key under it. One recipient, one wrapped key;
the sender's own address must be among them so they can read their own Sent
copy — `BuildEnvelope` does not add it implicitly, since only the caller
knows which recipient is the sender.

**3. Assemble the envelope** — `BuildEnvelope` (envelope.go)

Runs both steps and produces the same `Envelope` this package already knows
how to read: pass a built envelope through `ParseEnvelope` and it reads back
exactly, because it is the same struct.

### Why EncapsulateHybrid has no fixed vector

Every other mirror above reuses vectors already pinned for the decrypt
direction, because the underlying algorithms are symmetric and reversible:
AES-GCM and AES-KW are the same cipher run backwards, so a vector that proves
`DecryptSymmetrically`/`UnwrapKey` correct also proves their mirrors correct.
Regenerating `WrapKey`'s RFC 3394 vectors with noble's own `aeskw(...).encrypt`
confirmed this byte for byte.

Encapsulate cannot work that way: it is **randomised** by design — an
ephemeral X25519 key plus ML-KEM's own randomness — so two calls against the
same public key never produce the same ciphertext, and there is no single
expected output to pin.

The check that stands in for one instead is a **round trip across a real
noble keypair**: `TestEncapsulateHybridMatchesNobleKey` (hybrid_test.go)
encapsulates in Go against a public key noble's own `keygen` produced, then
decapsulates the result with `DecapsulateHybrid` using the matching private
key — the same function `TestDecapsulateHybridMatchesNoble` already proved
agrees with noble on a real ciphertext. If the combiner or the byte layout in
`EncapsulateHybrid` disagreed with noble's, this would fail: the private key
is noble's, so decapsulation follows noble's exact algorithm, not Go's
assumption about what it should do.

## Verification

This is the part to read before trusting any of it.

A round-trip test written only in Go would prove nothing here. If the seed
expansion or the combiner were wrong, encryption and decryption would be wrong
_in the same way_, the test would pass, and the package still could not read a
single real email. Agreement with the JS client is the only correctness that
counts.

So the tests run against **vectors generated by the real JS libraries**,
`@noble/post-quantum` and `@noble/ciphers`:

- `hybrid_test.go` — noble's ciphertext and seed must produce noble's shared
  secret. One assertion covers the seed expansion, both KEM halves and the
  combiner, since any of the three being wrong changes the output.
- `envelope_test.go` — a complete envelope built by the JS side, sealed for two
  recipients, must decrypt to the exact plaintext that went in. The fixture is
  `testdata/encrypted_body.txt`.
- `symmetric_test.go` — AES-KW against the published vectors in **RFC 3394
  section 4**. Those are the authoritative vectors for the algorithm, and they
  are what makes a hand-written implementation trustworthy.

The fixtures use deterministic test keys and invented content. Nothing in them
is sensitive.

## Files

| File           | What it holds                                                                              |
| -------------- | ------------------------------------------------------------------------------------------- |
| `core.go`      | Package doc, plus the JS reference implementation kept verbatim in a comment                |
| `symmetric.go` | `aesGCM` (unexported), and the package-level `DecryptSymmetrically`/`EncryptSymmetrically` that wrap it |
| `keywrap.go`   | `aesKeyWrap` (unexported, RFC 3394), and the package-level `UnwrapKey`/`WrapKey` that wrap it |
| `hybrid.go`    | `xwingKEM` (unexported), and the package-level `DecapsulateHybrid`/`EncapsulateHybrid`/`DecryptKeysHybrid`/`EncryptKeysHybrid` that wrap it |
| `email.go`     | `DecryptEmail`/`EncryptEmail` — the three ciphertexts under one session key                  |
| `envelope.go`  | `IsEncryptedBody`, `ParseEnvelope`, `DecryptEnvelope`, `BuildEnvelope`                       |

Each algorithm — AES-GCM, AES Key Wrap, X-Wing — is its own unexported type,
holding only what that algorithm needs and exposing only the operation it
performs (`Seal`/`Open`, `Wrap`/`Unwrap`, `Encapsulate`/`Decapsulate`). The
package-level functions above are thin wrappers kept for the rest of the
codebase to call; nothing outside this package constructs these types
directly. The point of the split: moving one of these algorithms to a package
of its own, if that is ever needed, is moving its file — the boundary is
already there.

## If you change something here

The JS library is the specification. When the two disagree, this package is
wrong.

Regenerate the vectors only against the real noble libraries, and never adjust an
expected value to make a test pass — a failing interop test means the port
diverged, which is exactly what these tests exist to catch.
