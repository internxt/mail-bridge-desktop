package store

const (
	KeyToken = "token"
	// KeyStoragePassphrase encrypts Gluon's message cache on disk. Losing it
	// makes that cache unreadable, so it is stored rather than regenerated.
	KeyStoragePassphrase = "storagePassphrase"
)
