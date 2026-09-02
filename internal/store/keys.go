package store

// Keys the bridge stores. They are named here so every package refers to the
// same strings.
//
// There are few of them on purpose: the account's session arrives over the
// control channel and lives in memory, so the only things worth keeping are
// those the parent does not send.
const (
	KeyToken = "token"
	// KeyStoragePassphrase encrypts Gluon's message cache on disk. Losing it
	// makes that cache unreadable, so it is stored rather than regenerated.
	KeyStoragePassphrase = "storagePassphrase"
)
