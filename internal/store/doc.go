// Package store keeps the daemon's sensitive values across restarts.
//
// It is a plain key-value store: it has no opinion about what a token or a
// keystore is, so a value fetched from the API and one pushed by the desktop
// client are saved the same way.
//
// Where a value ends up depends only on its size. Small values go to the
// system keychain; larger ones are encrypted with AES-GCM into stateDir, with
// their encryption key kept in the keychain. Nothing sensitive is ever written
// to disk in the clear.
package store
