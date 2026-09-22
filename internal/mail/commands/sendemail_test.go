package commands

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"testing"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/crypto"
)

// testRecipientPublicKeyHex is a real X-Wing public key, generated with
// noble's keygen from a seed of all-0x01 bytes — the same keypair
// internal/crypto's own tests use as "Alice" (see envelope_test.go), so
// BuildEnvelope really seals something that could be opened.
const testRecipientPublicKeyHex = "ec7b50cddc8360f98b189bac73d395ef947b37d8453886a253269f7b18b9eb78c1b63212471a0f979793f9936b3f496f4b5394ea69c2a35729f91c688f6bbbb864cd5e87108676c4014c2ba98204f911becae33a71e832ac012bb827578810955f8c6e2d26c0b17b7ba574990884546ba58bf6785721f3854f434cfea602e8595c71642e8d4c70934b7e54c638f5a13e1a136bc86565e6b40abc163ca65650baf953de7bb99b138ac1b695023103c9b417853c9d42e54fdb816174659d85a783e3d4613db1cbbaa63fb667a4a636804b6c4ae821ac5d6556688bab1dc10d6779b485c63c0ddacb91837c4ff3402e6214188072b4186a39c65bde524c683c95d3c8b65e37104f551b6a3602eda50b787182d703ac6a221428b4553e3b99c2b251ef642e31256c329b21d1246a71456fce700d7f50cfe5390a1c37bc133809f102c22914a1402c205c0512b733afeea04411ca5ebb0bca9392b1ee23935eb196024732daa2a1f79358e6e74b73c965a9e74778dc6921442b19328f6216a5e814ccc0639a863a437a614def5a61f38852151011b04a37bbc78c1eba4d8d1b3a1622a0dff74d25c731abb2a5fe5919f835bd3dd97330cbb7dba0b74260c963402160c4017d92256a3713c9e77ea0f4901accbd38715511784c9ec287dd85a769e081854b32aba9322a3840f6065133228c41851afcb40ea509cfbb86145fb8853ce14c649691136b8660b0077f3b2f9da82d483c1414c39a9777665899131a8336fb828480986df102628d10b54239cc20231457d4bbb7016f76029661f14ffd3532e2f8494e1613430730ab915683c3c8c4db2b4373a3057a097e23333605398b15cc4d6ac3fbd0732f21026bb0cd51fb738a740467114e7c66256b830022f28c028392cff8013d617c77a47bbda11c4a522f8f2b49f2822cc06338605671fca4518df9b3c506532c9cca3175330f8733ce11cb3fd8b95239ceebc9483cb68bff43b622911fcf4a9c57c226caa38bf0b081535999f573016b14563ec4826dc281dbabc633868a1d903d59207fc662a293735085c01f40b5b56cbb795ecabfad709d611cac73eaca579768213c18c969c59be58fcef6bdd8a85192907cd0773f81eaa24be07e0d620e9685acb0c6b0f54b47dffb510384241c4b733fe08dacb2852b2b74cc014e974a5e9db35d80d7b83ad31da1487a0170ba7fbc1c551a6f1eecb572084180b256962748d5e3200b731ac7c3928585a153b167c92a48cd91668c773707c054af16aa7bfacaa161a620600e8d08cc97601a53391da0247e5fca60cd1bb65ec0417177a9eb78cde5aa1dfae34e948417b3cc0b223803f5f40e8ae3a382848ff80c4185824076423ae4c137bd30bd81f04095c20a01e0a49f664f8f2b7f6bf6a990993cbc0596a514ccebc578c6418e825903ae11ac52831c6a48c67727409ed7274eea03eef32094271b02d4535563aa4924a2666871a4b690540c78b06043bca31ca4e42a03650ecab74792017217d10615d0acdee124e3222c90d79362207e4f7779e097501cf140b1a3431ee0cf27b23a50373d59976d82b5b1ce165f4aa1361157afad564081c85777584dd6058a1a4663b53234d7264fbac6877351d1928c6780f77d47209337271e305370df9aeffb74d7c75de55c006e2b2a979aaa76aaed9e76fa61e2a0a9aff50c054b3f819ee2da1cc9134008b9f5ec05"

// testRecipientPrivateKeyHex is the seed that keypair was generated from, so
// a test can open what SendEmail sealed.
const testRecipientPrivateKeyHex = "0101010101010101010101010101010101010101010101010101010101010101"

func testRecipientPublicKey(t *testing.T) []byte {
	t.Helper()
	key, err := hex.DecodeString(testRecipientPublicKeyHex)
	if err != nil {
		t.Fatalf("bad hex in test: %v", err)
	}
	return key
}

func testRecipientPublicKeyBase64(t *testing.T) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(testRecipientPublicKey(t))
}

func addr(email string) api.EmailAddressDto { return api.EmailAddressDto{Email: email} }

func TestSendEmailToAllInternxtRecipientsIsEncryptedAndINTERNXT(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{
			{Address: "bob@inxt.eu", PublicKey: &publicKey},
		},
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "hola",
		TextBody: "cuerpo del mensaje",
		To:       []api.EmailAddressDto{addr("bob@inxt.eu")},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	if !client.sendCalled {
		t.Fatal("SendEmail did not reach the client")
	}
	sent := client.sentEmail
	if sent.DeliveryMode == nil || *sent.DeliveryMode != api.SendEmailRequestDtoDeliveryModeINTERNXT {
		t.Errorf("deliveryMode = %v, want INTERNXT", sent.DeliveryMode)
	}
	if sent.Encryption == nil {
		t.Fatal("expected the message to carry an encryption block")
	}
	if len(sent.Encryption.WrappedKeys) != 1 {
		t.Fatalf("wrapped keys = %d, want 1 (no sender public key was given)", len(sent.Encryption.WrappedKeys))
	}
	if sent.Encryption.WrappedKeys[0].EncryptedForEmail != "bob@inxt.eu" {
		t.Errorf("wrapped for %q, want bob@inxt.eu", sent.Encryption.WrappedKeys[0].EncryptedForEmail)
	}
}

// sealedBody opens an encryption block a command actually produced, so a test
// can assert on the body that travelled rather than only on the call being
// made. It takes the block itself so both the send and the draft paths can
// use it.
func sealedBody(t *testing.T, block *api.EncryptionBlockDto, privateKeyHex, address string) crypto.Email {
	t.Helper()

	if block == nil {
		t.Fatal("expected an encryption block, got none")
	}

	privateKey, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		t.Fatalf("bad hex in test: %v", err)
	}

	wrappedKeys := make([]crypto.WrappedKey, 0, len(block.WrappedKeys))
	for _, key := range block.WrappedKeys {
		wrappedKeys = append(wrappedKeys, crypto.WrappedKey{
			EncryptedForEmail: key.EncryptedForEmail,
			EncryptedKey:      key.EncryptedKey,
			HybridCiphertext:  key.HybridCiphertext,
		})
	}

	email, err := crypto.DecryptEnvelope(crypto.Envelope{
		Version:                        block.Version,
		EncryptedText:                  block.EncryptedText,
		EncryptedPreview:               block.EncryptedPreview,
		EncryptedAttachmentsSessionKey: block.EncryptedAttachmentsSessionKey,
		WrappedKeys:                    wrappedKeys,
	}, privateKey, address)
	if err != nil {
		t.Fatalf("DecryptEnvelope: %v", err)
	}
	return email
}

// TestSendEmailSealsTheHTMLBody is the regression guard for a body that went
// missing: the envelope carries one body, read back as HTML by decryptBody,
// so an HTML message has to seal its HTML rather than its plain-text
// alternative.
func TestSendEmailSealsTheHTMLBody(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{{Address: "bob@inxt.eu", PublicKey: &publicKey}},
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "hola",
		TextBody: "texto plano",
		HTMLBody: "<p>cuerpo en HTML</p>",
		To:       []api.EmailAddressDto{addr("bob@inxt.eu")},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	body := sealedBody(t, client.sentEmail.Encryption, testRecipientPrivateKeyHex, "bob@inxt.eu")
	if body.Text != "<p>cuerpo en HTML</p>" {
		t.Errorf("sealed body = %q, want the HTML body", body.Text)
	}
	// The preview stays plain text: it is shown as a bare snippet.
	if body.Preview != "texto plano" {
		t.Errorf("sealed preview = %q, want the plain-text body", body.Preview)
	}
}

// TestSendEmailSealsAnHTMLOnlyMessage covers a client that sends no plain
// text at all: the body must still travel, and the preview falls back to it.
func TestSendEmailSealsAnHTMLOnlyMessage(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{{Address: "bob@inxt.eu", PublicKey: &publicKey}},
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "hola",
		HTMLBody: "<p>solo HTML</p>",
		To:       []api.EmailAddressDto{addr("bob@inxt.eu")},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	body := sealedBody(t, client.sentEmail.Encryption, testRecipientPrivateKeyHex, "bob@inxt.eu")
	if body.Text != "<p>solo HTML</p>" {
		t.Errorf("sealed body = %q, want the HTML body", body.Text)
	}
	if body.Preview == "" {
		t.Error("an HTML-only message sealed an empty preview")
	}
}

// TestSendEmailSealsAndUploadsAttachments is the point of the whole feature:
// the file must leave encrypted, and under the very key the envelope hands the
// recipient, or nobody will be able to open it.
func TestSendEmailSealsAndUploadsAttachments(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{{Address: "bob@inxt.eu", PublicKey: &publicKey}},
	}

	content := []byte("el contenido del adjunto")
	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "hola",
		TextBody: "cuerpo",
		To:       []api.EmailAddressDto{addr("bob@inxt.eu")},
		Attachments: []OutgoingAttachment{{
			Name:        "notas.txt",
			ContentType: "text/plain",
			Content:     content,
		}},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	if len(client.uploaded) != 1 {
		t.Fatalf("uploaded %d attachments, want 1", len(client.uploaded))
	}
	uploaded := client.uploaded[0]

	if bytes.Equal(uploaded.content, content) {
		t.Fatal("the attachment travelled in the clear")
	}
	if uploaded.name != "notas.txt" || uploaded.contentType != "text/plain" {
		t.Errorf("uploaded %q (%s), want notas.txt (text/plain)", uploaded.name, uploaded.contentType)
	}

	// The recipient opens the envelope for the key, then the file with it —
	// exactly what DownloadAttachments does when reading.
	sealed := sealedBody(t, client.sentEmail.Encryption, testRecipientPrivateKeyHex, "bob@inxt.eu")
	if len(sealed.AttachmentsSessionKey) == 0 {
		t.Fatal("the envelope carries no attachments session key")
	}

	opened, err := crypto.DecryptSymmetrically(sealed.AttachmentsSessionKey, uploaded.content, nil)
	if err != nil {
		t.Fatalf("the recipient cannot open the attachment: %v", err)
	}
	if !bytes.Equal(opened, content) {
		t.Errorf("attachment opened as %q, want %q", opened, content)
	}
}

// TestSendEmailReferencesTheUploadedAttachment checks the email actually
// points at what was stored; a blob nobody references is a file that never
// arrives.
func TestSendEmailReferencesTheUploadedAttachment(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{{Address: "bob@inxt.eu", PublicKey: &publicKey}},
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "hola",
		TextBody: "cuerpo",
		To:       []api.EmailAddressDto{addr("bob@inxt.eu")},
		Attachments: []OutgoingAttachment{
			{Name: "uno.txt", ContentType: "text/plain", Content: []byte("uno")},
			{Name: "dos.txt", ContentType: "text/plain", Content: []byte("dos")},
		},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	if client.sentEmail.Attachments == nil {
		t.Fatal("the email references no attachments")
	}
	refs := *client.sentEmail.Attachments
	if len(refs) != 2 {
		t.Fatalf("got %d references, want 2", len(refs))
	}
	if refs[0].BlobId == "" || refs[0].Name != "uno.txt" {
		t.Errorf("first reference = %+v, want uno.txt with a blob id", refs[0])
	}
	if refs[0].BlobId == refs[1].BlobId {
		t.Error("both attachments reference the same blob")
	}
}

// TestSendEmailStopsWhenAnAttachmentFails keeps a message from arriving
// without the file its author attached: the client is told instead, and still
// has the message to retry.
func TestSendEmailStopsWhenAnAttachmentFails(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{{Address: "bob@inxt.eu", PublicKey: &publicKey}},
		uploadErr:     errors.New("the upload allowance is exhausted"),
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "hola",
		TextBody: "cuerpo",
		To:       []api.EmailAddressDto{addr("bob@inxt.eu")},
		Attachments: []OutgoingAttachment{{
			Name: "notas.txt", ContentType: "text/plain", Content: []byte("x"),
		}},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err == nil {
		t.Fatal("expected the failed upload to stop the send")
	}
	if client.sendCalled {
		t.Error("the email was sent even though its attachment never made it")
	}
}

// TestSendEmailWithoutAttachmentsUploadsNothing keeps ordinary mail off the
// upload endpoint.
func TestSendEmailWithoutAttachmentsUploadsNothing(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{{Address: "bob@inxt.eu", PublicKey: &publicKey}},
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "hola",
		TextBody: "cuerpo",
		To:       []api.EmailAddressDto{addr("bob@inxt.eu")},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	if len(client.uploaded) != 0 {
		t.Errorf("uploaded %d attachments, want none", len(client.uploaded))
	}
	if client.sentEmail.Attachments != nil {
		t.Error("an email with no attachments should not reference any")
	}
}

// TestSendEmailRepliesThroughTheReplyEndpoint is what puts the answer in the
// conversation: the ordinary send endpoint ignores the email being replied to,
// so a reply has to go through its own.
func TestSendEmailRepliesThroughTheReplyEndpoint(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{{Address: "bob@inxt.eu", PublicKey: &publicKey}},
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:          "Re: hola",
		TextBody:         "respuesta",
		InReplyToEmailID: "M1",
		To:               []api.EmailAddressDto{addr("bob@inxt.eu")},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	if client.sendCalled {
		t.Error("a reply went through the plain send endpoint, so it would start its own thread")
	}
	if client.repliedTo != "M1" {
		t.Errorf("replied to %q, want M1", client.repliedTo)
	}

	// A reply is still sealed like any other message.
	if client.sentReply.Encryption == nil {
		t.Fatal("the reply carries no encryption block")
	}
	body := sealedBody(t, client.sentReply.Encryption, testRecipientPrivateKeyHex, "bob@inxt.eu")
	if body.Text != "respuesta" {
		t.Errorf("sealed body = %q, want the reply body", body.Text)
	}
}

// TestSendEmailRepliesWithTheComposedSubjectAndRecipients keeps what the user
// wrote: the backend can derive both from the original, but the client already
// resolved them and showing something else would surprise the sender.
func TestSendEmailRepliesWithTheComposedSubjectAndRecipients(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{
			{Address: "bob@inxt.eu", PublicKey: &publicKey},
			{Address: "carol@inxt.eu", PublicKey: &publicKey},
		},
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:          "Re: hola",
		TextBody:         "respuesta",
		InReplyToEmailID: "M1",
		To:               []api.EmailAddressDto{addr("bob@inxt.eu")},
		Cc:               []api.EmailAddressDto{addr("carol@inxt.eu")},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	reply := client.sentReply
	if reply.Subject == nil || *reply.Subject != "Re: hola" {
		t.Errorf("subject = %v, want the one the client composed", reply.Subject)
	}
	if reply.To == nil || len(*reply.To) != 1 || (*reply.To)[0].Email != "bob@inxt.eu" {
		t.Errorf("to = %v, want [bob@inxt.eu]", reply.To)
	}
	if reply.Cc == nil || len(*reply.Cc) != 1 {
		t.Errorf("cc = %v, want [carol@inxt.eu]", reply.Cc)
	}
}

// TestSendEmailWithoutAReplyIdUsesTheSendEndpoint is the other half: an
// ordinary message must not be filed into somebody else's conversation.
func TestSendEmailWithoutAReplyIdUsesTheSendEndpoint(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{{Address: "bob@inxt.eu", PublicKey: &publicKey}},
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "hola",
		TextBody: "cuerpo",
		To:       []api.EmailAddressDto{addr("bob@inxt.eu")},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	if !client.sendCalled {
		t.Error("an ordinary message did not reach the send endpoint")
	}
	if client.repliedTo != "" {
		t.Errorf("it replied to %q, want nothing", client.repliedTo)
	}
}

func TestSendEmailIncludesTheSendersOwnWrappedKey(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{{Address: "bob@inxt.eu", PublicKey: &publicKey}},
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "hola",
		TextBody: "cuerpo",
		To:       []api.EmailAddressDto{addr("bob@inxt.eu")},
	}, Account{Address: "alice@inxt.eu", PublicKey: testRecipientPublicKey(t)}, nil)
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	if len(client.sentEmail.Encryption.WrappedKeys) != 2 {
		t.Fatalf("wrapped keys = %d, want 2 (recipient + sender)", len(client.sentEmail.Encryption.WrappedKeys))
	}
}

func TestSendEmailWithAnExternalRecipientUsesServerKeyAndEXTERNAL(t *testing.T) {
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{
			{Address: "carol@example.com", PublicKey: nil},
		},
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "hola",
		TextBody: "cuerpo",
		To:       []api.EmailAddressDto{addr("carol@example.com")},
	}, Account{Address: "alice@inxt.eu"}, testRecipientPublicKey(t))
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	sent := client.sentEmail
	if sent.DeliveryMode == nil || *sent.DeliveryMode != api.SendEmailRequestDtoDeliveryModeEXTERNAL {
		t.Errorf("deliveryMode = %v, want EXTERNAL", sent.DeliveryMode)
	}
	if sent.Encryption == nil || len(sent.Encryption.WrappedKeys) != 1 {
		t.Fatal("expected the external recipient to still be sealed, with the server key")
	}
}

func TestSendEmailWithoutServerKeyFailsForExternalRecipients(t *testing.T) {
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{{Address: "carol@example.com", PublicKey: nil}},
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "hola",
		TextBody: "cuerpo",
		To:       []api.EmailAddressDto{addr("carol@example.com")},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err == nil {
		t.Fatal("expected an error: no server key to seal an external recipient with")
	}
	if client.sendCalled {
		t.Error("SendEmail should not have reached the client")
	}
}

func TestSendEmailPropagatesLookupFailure(t *testing.T) {
	client := &fakeClient{lookupErr: errors.New("api is down")}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject: "hola",
		To:      []api.EmailAddressDto{addr("bob@inxt.eu")},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err == nil {
		t.Fatal("expected the lookup failure to propagate")
	}
}

func TestSendEmailPropagatesSendFailure(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{{Address: "bob@inxt.eu", PublicKey: &publicKey}},
		sendErr:       errors.New("api is down"),
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject: "hola",
		To:      []api.EmailAddressDto{addr("bob@inxt.eu")},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err == nil {
		t.Fatal("expected the send failure to propagate")
	}
}

func TestSendEmailRequiresAtLeastOneRecipient(t *testing.T) {
	client := &fakeClient{}

	if _, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{Subject: "hola"}, Account{}, nil); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// TestSendEmailDeduplicatesAcrossToAndCc keeps a recipient in both To and Cc
// from being looked up, and sealed for, twice.
func TestSendEmailDeduplicatesAcrossToAndCc(t *testing.T) {
	publicKey := testRecipientPublicKeyBase64(t)
	client := &fakeClient{
		recipientKeys: []api.RecipientKeyDto{{Address: "bob@inxt.eu", PublicKey: &publicKey}},
	}

	_, err := SendEmail(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "hola",
		TextBody: "cuerpo",
		To:       []api.EmailAddressDto{addr("bob@inxt.eu")},
		Cc:       []api.EmailAddressDto{addr("BOB@inxt.eu")},
	}, Account{Address: "alice@inxt.eu"}, nil)
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}

	if len(client.sentEmail.Encryption.WrappedKeys) != 1 {
		t.Fatalf("wrapped keys = %d, want 1 (same address in To and Cc)", len(client.sentEmail.Encryption.WrappedKeys))
	}
}
