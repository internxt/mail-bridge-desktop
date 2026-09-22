package store

// Keys the bridge stores.
//
// The account's session arrives from the parent over the control channel and
// lives in memory, so what is kept here is only what the parent does not send
// and the bridge cannot afford to make up again on the next start.
const (
	// KeyStoragePassphrase encrypts Gluon's message cache on disk. Losing it
	// makes that cache unreadable, so it is stored rather than regenerated.
	KeyStoragePassphrase = "storagePassphrase"
	KeyTLSCertificate    = "tlsCertificate"
	KeyTLSPrivateKey     = "tlsPrivateKey"
)
