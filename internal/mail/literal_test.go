package mail

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"mime/quotedprintable"
	"os"
	"strings"
	"testing"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/logger"
)

// The fixture is a real encrypted body built with the JS libraries the web
// client uses, sealed for the address below.
const (
	testAddress    = "alice@inxt.me"
	testPrivateKey = "0101010101010101010101010101010101010101010101010101010101010101"
	testPlaintext  = "Hola, esto es un correo cifrado con acentos: ñ á é €"
)

type fakeClient struct {
	thread []api.EmailResponseDto
	err    error
}

func (f *fakeClient) GetUserFolder(ctx context.Context, token string, opts api.ListEmailsOptions) (api.EmailListResponseDto, error) {
	return api.EmailListResponseDto{}, f.err
}

func (f *fakeClient) GetMailboxes(ctx context.Context, token string) ([]api.MailboxResponseDto, error) {
	return nil, f.err
}

func (f *fakeClient) GetThread(ctx context.Context, token, emailID string) ([]api.EmailResponseDto, error) {
	return f.thread, f.err
}

func encryptedEmail(t *testing.T) api.EmailResponseDto {
	t.Helper()
	body, err := os.ReadFile("commands/testdata/encrypted_body.txt")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	text := string(body)
	return api.EmailResponseDto{
		Id:         "M1",
		Subject:    "cifrado",
		From:       []api.EmailAddressDto{{Email: "bob@inxt.me"}},
		ReceivedAt: "2025-06-15T10:30:00Z",
		TextBody:   &text,
	}
}

func testService(t *testing.T, client *fakeClient, privateKey string) *MailService {
	t.Helper()

	var key []byte
	if privateKey != "" {
		decoded, err := hex.DecodeString(privateKey)
		if err != nil {
			t.Fatalf("bad hex in test: %v", err)
		}
		key = decoded
	}

	return New(client, Account{
		Token:      "tok",
		Address:    testAddress,
		PrivateKey: key,
	}, logger.New("test"))
}

// decodeQuotedPrintable undoes the transfer encoding BuildLiteral applies, so
// a test can look for the plaintext it expects. Accented characters arrive
// encoded, which is exactly why this is needed.
func decodeQuotedPrintable(t *testing.T, literal []byte) string {
	t.Helper()
	decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(string(literal))))
	if err != nil {
		t.Fatalf("decode quoted-printable: %v", err)
	}
	return string(decoded)
}

// TestGetMessageLiteralDecryptsBody is the check that the whole chain is wired:
// an encrypted email fetched through the service reaches the client readable.
func TestGetMessageLiteralDecryptsBody(t *testing.T) {
	client := &fakeClient{thread: []api.EmailResponseDto{encryptedEmail(t)}}
	service := testService(t, client, testPrivateKey)

	literal, err := service.GetMessageLiteral(context.Background(), "M1")
	if err != nil {
		t.Fatalf("GetMessageLiteral: %v", err)
	}
	if strings.Contains(string(literal), "INTERNXT-ENCRYPTED-EMAIL-v1") {
		t.Fatal("the literal still carries the envelope")
	}
	if !strings.Contains(decodeQuotedPrintable(t, literal), testPlaintext) {
		t.Fatalf("the plaintext is missing from the literal:\n%s", literal)
	}
}

// TestGetMessageLiteralServesWhatItCannotDecrypt covers a message sealed for
// another address: it is still built into a literal, and the error explains
// why it is unreadable rather than replacing the message.
func TestGetMessageLiteralServesWhatItCannotDecrypt(t *testing.T) {
	client := &fakeClient{thread: []api.EmailResponseDto{encryptedEmail(t)}}
	service := testService(t, client, "")

	literal, err := service.GetMessageLiteral(context.Background(), "M1")
	if err == nil {
		t.Fatal("expected an error explaining the body could not be decrypted")
	}
	if literal == nil {
		t.Fatal("expected the message to be served anyway")
	}
	if !strings.Contains(string(literal), "Subject: cifrado") {
		t.Fatalf("the message lost its headers:\n%s", literal)
	}
}

// TestGetMessageLiteralFailsWhenTheEmailIsMissing keeps a transport failure
// distinguishable from an undecryptable body.
func TestGetMessageLiteralFailsWhenTheEmailIsMissing(t *testing.T) {
	client := &fakeClient{err: errors.New("boom")}
	service := testService(t, client, testPrivateKey)

	literal, err := service.GetMessageLiteral(context.Background(), "M1")
	if err == nil {
		t.Fatal("expected an error")
	}
	if literal != nil {
		t.Fatalf("expected no literal, got %q", literal)
	}
}
