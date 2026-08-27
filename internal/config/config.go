package config

import (
	"net"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Host       string
	IMAPAddr   string
	SMTPAddr   string
	SMTPDomain string
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() Config {
	// A missing .env file is normal in deployed environments, where values are
	// supplied by the service manager or process environment instead.
	_ = godotenv.Load()

	host := env("BRIDGE_HOST", "127.0.0.1")
	return Config{
		Host:       host,
		IMAPAddr:   net.JoinHostPort(host, env("BRIDGE_IMAP_PORT", "1143")),
		SMTPAddr:   net.JoinHostPort(host, env("BRIDGE_SMTP_PORT", "2025")),
		SMTPDomain: env("BRIDGE_SMTP_DOMAIN", "localhost"),
	}
}
