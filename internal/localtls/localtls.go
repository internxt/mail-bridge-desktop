// Package localtls provides the certificate the bridge's loopback IMAP and SMTP
// servers present to mail clients.
//
// A mail client will not send a password over a connection it cannot encrypt —
// Apple Mail refuses outright — so the servers need a certificate even though
// nothing ever leaves the machine. No authority can issue one for 127.0.0.1, so
// the bridge signs its own and the user is asked to trust it once.
package localtls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"

	"mail-bridge-desktop/internal/store"
)

const (
	validity    = 730 * 24 * time.Hour
	renewBefore = 30 * 24 * time.Hour
	keyBits     = 2048
	commonName  = "Internxt Mail Bridge"
)

// Ensure returns the TLS configuration both local servers present, along with the
// certificate in DER form for the parent to hand to the system.
func Ensure(credentials *store.Store) (*tls.Config, []byte, error) {
	if credentials == nil {
		return nil, nil, errors.New("localtls: credential store is required")
	}

	certificate, err := load(credentials)
	if err != nil {
		return nil, nil, err
	}
	if certificate == nil {
		if certificate, err = generate(credentials); err != nil {
			return nil, nil, err
		}
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*certificate},
		MinVersion:   tls.VersionTLS12,
	}, certificate.Certificate[0], nil
}

// load returns the stored certificate, or nil when there is none worth keeping.
// A pair that cannot be read is treated as absent: the only thing to do with it
// is replace it, and failing the bridge over a key it can rewrite helps nobody.
func load(credentials *store.Store) (*tls.Certificate, error) {
	certPEM, err := credentials.Get(store.KeyTLSCertificate)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("localtls: read certificate: %w", err)
	}

	keyPEM, err := credentials.Get(store.KeyTLSPrivateKey)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("localtls: read private key: %w", err)
	}

	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, nil
	}
	if !usable(&certificate) {
		return nil, nil
	}

	return &certificate, nil
}

// usable reports whether a certificate will still be accepted for long enough to
// be worth keeping.
func usable(certificate *tls.Certificate) bool {
	parsed, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return false
	}

	now := time.Now()
	return now.After(parsed.NotBefore) && now.Add(renewBefore).Before(parsed.NotAfter)
}

// generate creates the certificate and stores it. It is self-signed and marked as
// a certificate authority so that the system can be asked to trust it as a root,
// which is what spares the user a warning from every mail client.
func generate(credentials *store.Store) (*tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return nil, fmt.Errorf("localtls: generate private key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("localtls: generate serial number: %w", err)
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName, Organization: []string{"Internxt"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(validity),

		// macOS ignores the common name and reads these instead. Without the
		// address the client dialled, the connection fails with no explanation.
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("localtls: create certificate: %w", err)
	}

	encodedKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("localtls: encode private key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encodedKey})

	if err := credentials.Set(store.KeyTLSCertificate, certPEM); err != nil {
		return nil, fmt.Errorf("localtls: store certificate: %w", err)
	}
	if err := credentials.Set(store.KeyTLSPrivateKey, keyPEM); err != nil {
		return nil, fmt.Errorf("localtls: store private key: %w", err)
	}

	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("localtls: load the certificate just created: %w", err)
	}
	return &certificate, nil
}
