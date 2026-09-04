package commands

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/crypto"
)

// OutgoingMessage is a composed email, ready to send.
type OutgoingMessage struct {
	Subject  string
	HTMLBody string
	TextBody string
	To       []api.EmailAddressDto
	Cc       []api.EmailAddressDto
	Bcc      []api.EmailAddressDto
}

// SendEmail seals msg for every recipient and the sender's own address, then
// submits it.
//
// A recipient without an Internxt public key is sealed with serverPublicKey
// instead of left in the clear: the backend decrypts it before handing the
// message to whatever provider serves that address, so the body never travels
// over a connection this bridge does not control. Any such recipient makes the
// whole message deliveryMode EXTERNAL; Internxt addresses only make it
// INTERNXT.
func SendEmail(ctx context.Context, client Client, token string, msg OutgoingMessage, account Account, serverPublicKey []byte) error {
	addresses := uniqueAddresses(msg.To, msg.Cc, msg.Bcc)
	if len(addresses) == 0 {
		return fmt.Errorf("send email: no recipients")
	}

	keys, err := lookupKeys(ctx, client, token, addresses)
	if err != nil {
		return err
	}

	recipients, allInternxt, err := sealFor(addresses, keys, serverPublicKey)
	if err != nil {
		return err
	}
	if len(account.PublicKey) > 0 {
		recipients = append(recipients, crypto.Recipient{Address: account.Address, PublicKey: account.PublicKey})
	}

	envelope, err := crypto.BuildEnvelope(msg.body(), recipients)
	if err != nil {
		return fmt.Errorf("send email: seal message: %w", err)
	}

	deliveryMode := api.SendEmailRequestDtoDeliveryModeINTERNXT
	if !allInternxt {
		deliveryMode = api.SendEmailRequestDtoDeliveryModeEXTERNAL
	}

	block := toEncryptionBlock(envelope)
	_, err = client.SendEmail(ctx, token, api.SendEmailRequestDto{
		Subject:      msg.Subject,
		Encryption:   &block,
		To:           msg.To,
		Cc:           optionalAddresses(msg.Cc),
		Bcc:          optionalAddresses(msg.Bcc),
		DeliveryMode: &deliveryMode,
	})
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}

const previewPlaintextLength = 256

// body is what gets sealed. The envelope holds a single body, read back as
// HTML by decryptBody, so the HTML part is the one that travels when a client
// sent both — the same choice the web client makes. The preview stays plain
// text, since it is shown as a bare snippet in listings.
func (m OutgoingMessage) body() crypto.Email {
	text := m.HTMLBody
	if text == "" {
		text = m.TextBody
	}

	preview := m.TextBody
	if preview == "" {
		preview = m.HTMLBody
	}

	return crypto.Email{Text: text, Preview: snippet(preview)}
}

func snippet(text string) string {
	collapsed := strings.Join(strings.Fields(text), " ")
	if len(collapsed) > previewPlaintextLength {
		return collapsed[:previewPlaintextLength]
	}
	return collapsed
}

// sealFor pairs every address with the key to seal it under, falling back to
// the server's key for an address with none of its own. allInternxt reports
// whether every address had one, which is what tells an INTERNXT delivery
// from an EXTERNAL one.
func sealFor(addresses []string, keys map[string][]byte, serverPublicKey []byte) (recipients []crypto.Recipient, allInternxt bool, err error) {
	recipients = make([]crypto.Recipient, 0, len(addresses)+1)
	allInternxt = true

	for _, address := range addresses {
		publicKey, found := keys[strings.ToLower(address)]
		if !found {
			if len(serverPublicKey) == 0 {
				return nil, false, fmt.Errorf("send email: %s has no Internxt key and no server key is configured to seal it with", address)
			}
			publicKey = serverPublicKey
			allInternxt = false
		}
		recipients = append(recipients, crypto.Recipient{Address: address, PublicKey: publicKey})
	}
	return recipients, allInternxt, nil
}

// lookupKeys resolves the addresses that have an Internxt public key, keyed
// by their lowercased form. An address without one is simply absent.
func lookupKeys(ctx context.Context, client Client, token string, addresses []string) (map[string][]byte, error) {
	recipients, err := client.LookupRecipientKeys(ctx, token, addresses)
	if err != nil {
		return nil, fmt.Errorf("send email: look up recipient keys: %w", err)
	}

	keys := make(map[string][]byte, len(recipients))
	for _, recipient := range recipients {
		if recipient.PublicKey == nil || *recipient.PublicKey == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(*recipient.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("send email: decode public key for %s: %w", recipient.Address, err)
		}
		keys[strings.ToLower(recipient.Address)] = decoded
	}
	return keys, nil
}

func uniqueAddresses(groups ...[]api.EmailAddressDto) []string {
	seen := make(map[string]bool)
	var addresses []string
	for _, group := range groups {
		for _, address := range group {
			key := strings.ToLower(address.Email)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			addresses = append(addresses, address.Email)
		}
	}
	return addresses
}

func optionalAddresses(addresses []api.EmailAddressDto) *[]api.EmailAddressDto {
	if len(addresses) == 0 {
		return nil
	}
	return &addresses
}

func toEncryptionBlock(envelope crypto.Envelope) api.EncryptionBlockDto {
	wrappedKeys := make([]api.EncryptedWrappedKeyDto, 0, len(envelope.WrappedKeys))
	for _, key := range envelope.WrappedKeys {
		wrappedKeys = append(wrappedKeys, api.EncryptedWrappedKeyDto{
			EncryptedForEmail: key.EncryptedForEmail,
			EncryptedKey:      key.EncryptedKey,
			HybridCiphertext:  key.HybridCiphertext,
		})
	}

	return api.EncryptionBlockDto{
		Version:                        envelope.Version,
		EncryptedText:                  envelope.EncryptedText,
		EncryptedPreview:               envelope.EncryptedPreview,
		EncryptedAttachmentsSessionKey: envelope.EncryptedAttachmentsSessionKey,
		WrappedKeys:                    wrappedKeys,
	}
}
