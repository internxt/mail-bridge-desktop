package crypto

import "fmt"

type EncryptedEmail struct {
	Text                  []byte
	Preview               []byte
	AttachmentsSessionKey []byte
}

type Email struct {
	Text                  string
	Preview               string
	AttachmentsSessionKey []byte
}

// EncryptEmail is the mirror of DecryptEmail: seals the body, preview and
// attachments session key under one caller-supplied symmetric key.
func EncryptEmail(email Email, key, aux []byte) (EncryptedEmail, error) {
	var encrypted EncryptedEmail

	text, err := EncryptSymmetrically(key, []byte(email.Text), aux)
	if err != nil {
		return EncryptedEmail{}, fmt.Errorf("crypto: encrypt body: %w", err)
	}
	encrypted.Text = text

	preview, err := EncryptSymmetrically(key, []byte(email.Preview), aux)
	if err != nil {
		return EncryptedEmail{}, fmt.Errorf("crypto: encrypt preview: %w", err)
	}
	encrypted.Preview = preview

	if len(email.AttachmentsSessionKey) > 0 {
		attachmentsKey, err := EncryptSymmetrically(key, email.AttachmentsSessionKey, aux)
		if err != nil {
			return EncryptedEmail{}, fmt.Errorf("crypto: encrypt attachments session key: %w", err)
		}
		encrypted.AttachmentsSessionKey = attachmentsKey
	}

	return encrypted, nil
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
