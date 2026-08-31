package crypto

import "fmt"

type EncryptedEmail struct {
	Text                  []byte
	Preview               []byte
	AttachmentsSessionKey []byte
}

// Email is a decrypted email body.
type Email struct {
	Text                  string
	Preview               string
	AttachmentsSessionKey []byte
}

func DecryptEmail(encrypted EncryptedEmail, key, aux []byte) (Email, error) {
	var email Email

	// Decrypt the body
	if len(encrypted.Text) > 0 {
		text, err := DecryptSymmetrically(key, encrypted.Text, aux)
		if err != nil {
			return Email{}, fmt.Errorf("crypto: decrypt body: %w", err)
		}
		email.Text = string(text)
	}

	// Decrypt the preview
	if len(encrypted.Preview) > 0 {
		preview, err := DecryptSymmetrically(key, encrypted.Preview, aux)
		if err != nil {
			return Email{}, fmt.Errorf("crypto: decrypt preview: %w", err)
		}
		email.Preview = string(preview)
	}

	// Decrypt the attachments session key
	if len(encrypted.AttachmentsSessionKey) > 0 {
		sessionKey, err := DecryptSymmetrically(key, encrypted.AttachmentsSessionKey, aux)
		if err != nil {
			return Email{}, fmt.Errorf("crypto: decrypt attachments session key: %w", err)
		}
		email.AttachmentsSessionKey = sessionKey
	}

	return email, nil
}
