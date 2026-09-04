package config

import (
	"encoding/base64"
	"net"
	"os"

	"github.com/joho/godotenv"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Load() Config {
	_ = godotenv.Load()

	host := env("BRIDGE_HOST", "127.0.0.1")
	return Config{
		Host:            host,
		IMAPAddr:        net.JoinHostPort(host, env("BRIDGE_IMAP_PORT", "1143")),
		SMTPAddr:        net.JoinHostPort(host, env("BRIDGE_SMTP_PORT", "2025")),
		SMTPDomain:      env("BRIDGE_SMTP_DOMAIN", "localhost"),
		MailAPI:         env("MAIL_API_URL", ""),
		LogImapProtocol: env("BRIDGE_LOG_IMAP_PROTOCOL", "") == "true",
		ServerPublicKey: decodeServerPublicKey(env("MAIL_SERVER_PUBLIC_KEY", "")),
	}
}

func decodeServerPublicKey(encoded string) []byte {
	if encoded == "" {
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	return key
}
