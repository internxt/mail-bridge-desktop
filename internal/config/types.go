package config

type Config struct {
	Host            string
	IMAPAddr        string
	SMTPAddr        string
	SMTPDomain      string
	MailAPI         string
	LogImapProtocol bool
	ServerPublicKey []byte
}
