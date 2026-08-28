package config

import "testing"

func TestLoadUsesBridgeHost(t *testing.T) {
	t.Setenv("BRIDGE_HOST", "localhost")
	t.Setenv("BRIDGE_IMAP_PORT", "1144")
	t.Setenv("BRIDGE_SMTP_PORT", "2525")
	t.Setenv("BRIDGE_SMTP_DOMAIN", "bridge.test")

	cfg := Load()
	if cfg.Host != "localhost" {
		t.Fatalf("Host = %q, want localhost", cfg.Host)
	}
	if cfg.IMAPAddr != "localhost:1144" {
		t.Fatalf("IMAPAddr = %q, want localhost:1144", cfg.IMAPAddr)
	}
	if cfg.SMTPAddr != "localhost:2525" {
		t.Fatalf("SMTPAddr = %q, want localhost:2525", cfg.SMTPAddr)
	}
	if cfg.SMTPDomain != "bridge.test" {
		t.Fatalf("SMTPDomain = %q, want bridge.test", cfg.SMTPDomain)
	}
}
