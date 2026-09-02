package store

// Keys the bridge stores.
//
// There is only one: the account's session arrives from the parent over the
// control channel and lives in memory, so the sole thing worth keeping is what
// the parent does not send.
const (
	// KeyStoragePassphrase encrypts Gluon's message cache on disk. Losing it
	// makes that cache unreadable, so it is stored rather than regenerated.
	KeyStoragePassphrase = "storagePassphrase"
)
