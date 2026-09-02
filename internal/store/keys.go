package store

const (
	KeyToken = "token"
	KeyEmail = "email"

	// KeyEncryptionPrivateKey is the account's mail private key, already
	// decrypted by the desktop client: the 32-byte root seed the hybrid KEM
	// expands its component keys from, base64-encoded.
	KeyEncryptionPrivateKey = "encryptionPrivateKey"

	// KeyPublicKey is the account's hybrid (X25519 + ML-KEM-768) public key,
	// base64-encoded. Reading mail does not need it; sending does.
	KeyPublicKey = "publicKey"

	// KeyStoragePassphrase encrypts Gluon's message cache on disk. Losing it
	// makes that cache unreadable, so it is stored rather than regenerated.
	KeyStoragePassphrase = "storagePassphrase"
)
