package localtls

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"
	"time"

	"mail-bridge-desktop/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.NewForTesting(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return s
}

func ensure(t *testing.T, credentials *store.Store) ([]byte, *x509.Certificate) {
	t.Helper()

	_, der, err := Ensure(credentials)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse the certificate Ensure returned: %v", err)
	}
	return der, parsed
}

// TestEnsurePersists matters because the user is asked to trust this exact
// certificate from their mail client: a different one on the next start means a
// new warning, or a client that stops connecting altogether.
func TestEnsurePersists(t *testing.T) {
	credentials := newTestStore(t)

	first, _ := ensure(t, credentials)
	second, _ := ensure(t, credentials)

	if !bytes.Equal(first, second) {
		t.Error("the certificate changed between calls; the user would have to trust it again")
	}
}

// TestCertificateMeetsApplePolicy covers the requirements macOS applies to any
// TLS server certificate. Failing one of them shows up as a connection that
// fails with no explanation, so they are worth asserting rather than assuming.
func TestCertificateMeetsApplePolicy(t *testing.T) {
	_, certificate := ensure(t, newTestStore(t))

	if lifetime := certificate.NotAfter.Sub(certificate.NotBefore); lifetime > 825*24*time.Hour {
		t.Errorf("the certificate lasts %v; macOS refuses to trust more than 825 days", lifetime)
	}

	if !containsExtKeyUsage(certificate.ExtKeyUsage, x509.ExtKeyUsageServerAuth) {
		t.Error("the certificate has no serverAuth extended key usage")
	}

	if !certificate.IsCA || !certificate.BasicConstraintsValid {
		t.Error("the certificate cannot be installed as a trusted root")
	}

	if err := certificate.VerifyHostname("127.0.0.1"); err != nil {
		t.Errorf("the certificate does not cover the address clients dial: %v", err)
	}
	if err := certificate.VerifyHostname("localhost"); err != nil {
		t.Errorf("the certificate does not cover localhost: %v", err)
	}
	if !containsIP(certificate.IPAddresses, net.IPv6loopback) {
		t.Error("the certificate does not cover the IPv6 loopback address")
	}
}

// TestEnsureReplacesAnExpiredCertificate keeps a two-year certificate from
// taking the bridge down on the day it lapses.
func TestEnsureReplacesAnExpiredCertificate(t *testing.T) {
	credentials := newTestStore(t)
	original, _ := ensure(t, credentials)

	expire(t, credentials)

	replacement, certificate := ensure(t, credentials)
	if bytes.Equal(original, replacement) {
		t.Fatal("the expired certificate was kept")
	}
	if time.Now().After(certificate.NotAfter) {
		t.Error("the replacement is expired too")
	}
}

// TestEnsureReplacesAnUnreadablePair covers a stored value that cannot be parsed
// back: there is nothing to do with it but replace it, and refusing to start
// would leave the user with a bridge no restart could fix.
func TestEnsureReplacesAnUnreadablePair(t *testing.T) {
	credentials := newTestStore(t)
	if _, _, err := Ensure(credentials); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if err := credentials.Set(store.KeyTLSCertificate, []byte("not a certificate")); err != nil {
		t.Fatalf("overwrite the certificate: %v", err)
	}

	if _, _, err := Ensure(credentials); err != nil {
		t.Fatalf("Ensure after corruption: %v", err)
	}
}

func TestEnsureRequiresAStore(t *testing.T) {
	if _, _, err := Ensure(nil); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// expire rewrites the stored certificate as one that lapsed yesterday, so the
// renewal path can be exercised without waiting two years for it.
func expire(t *testing.T, credentials *store.Store) {
	t.Helper()

	certPEM, err := credentials.Get(store.KeyTLSCertificate)
	if err != nil {
		t.Fatalf("read the stored certificate: %v", err)
	}
	keyPEM, err := credentials.Get(store.KeyTLSPrivateKey)
	if err != nil {
		t.Fatalf("read the stored private key: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	template, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse the stored certificate: %v", err)
	}
	template.NotBefore = time.Now().Add(-2 * time.Hour)
	template.NotAfter = time.Now().Add(-time.Hour)

	keyBlock, _ := pem.Decode(keyPEM)
	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		t.Fatalf("parse the stored private key: %v", err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		t.Fatal("the stored key cannot sign")
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, signer.Public(), signer)
	if err != nil {
		t.Fatalf("re-sign the certificate as expired: %v", err)
	}
	expired := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := credentials.Set(store.KeyTLSCertificate, expired); err != nil {
		t.Fatalf("store the expired certificate: %v", err)
	}
}

func containsExtKeyUsage(usages []x509.ExtKeyUsage, want x509.ExtKeyUsage) bool {
	for _, usage := range usages {
		if usage == want {
			return true
		}
	}
	return false
}

func containsIP(addresses []net.IP, want net.IP) bool {
	for _, address := range addresses {
		if address.Equal(want) {
			return true
		}
	}
	return false
}
