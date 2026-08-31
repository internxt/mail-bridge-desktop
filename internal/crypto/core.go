// Package crypto ports the parts of Internxt's JS crypto library that the
// bridge needs in order to read encrypted mail.
//
// The account's own private key arrives from the client via IPC,
// so it has to go through DecryptSymmetrically first; what comes
// out is the 32-byte root seed the hybrid KEM expands its component keys from.
//
// Correctness here is only meaningful relative to the JS client: a self-
// consistent implementation that disagrees with it cannot read any real mail.
// That is what the noble-generated vectors in hybrid_test.go pin down.
package crypto
