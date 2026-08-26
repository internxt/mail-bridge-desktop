package config

import "os"

type Config struct {
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
	return Config{
		SMTPAddr:   "127.0.0.1:" + env("BRIDGE_SMTP_PORT", "2025"),
		SMTPDomain: env("BRIDGE_SMTP_DOMAIN", "localhost"),
	}
}
