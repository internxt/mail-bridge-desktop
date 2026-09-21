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

// TestLoadKeepsTLSOffUnlessAsked matters because this daemon is shared: the Windows and
// Linux apps run it too, and they have no way yet to make their system trust the
// certificate it would serve. Defaulting to on would hand their users a warning, and
// withdraw SMTP AUTH from the clients they already have configured.
func TestLoadKeepsTLSOffUnlessAsked(t *testing.T) {
	if cfg := Load(); cfg.TLS {
		t.Error("TLS is on with no BRIDGE_TLS set; a parent that never asked would start serving a certificate")
	}

	t.Setenv("BRIDGE_TLS", "true")
	if cfg := Load(); !cfg.TLS {
		t.Error("BRIDGE_TLS=true did not turn TLS on")
	}

	t.Setenv("BRIDGE_TLS", "1")
	if cfg := Load(); cfg.TLS {
		t.Error("only \"true\" should turn TLS on, so a half-set variable cannot enable it by accident")
	}
}
