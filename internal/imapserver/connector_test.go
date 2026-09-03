package imapserver

import (
	"context"
	"errors"
	"testing"

	"github.com/ProtonMail/gluon/imap"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/logger"
)

type fakeMailService struct {
	literal   []byte
	err       error
	forgotten int
}

func (f *fakeMailService) ListMailboxes(ctx context.Context) ([]api.MailboxResponseDto, error) {
	return nil, nil
}

func (f *fakeMailService) ListAllEmails(ctx context.Context, opts api.ListEmailsOptions) ([]api.EmailSummaryResponseDto, error) {
	return nil, nil
}

func (f *fakeMailService) GetMessageLiteral(ctx context.Context, emailID string) ([]byte, error) {
	return f.literal, f.err
}

func (f *fakeMailService) ForgetThreads() { f.forgotten++ }

func testConnector(service MailService) *mailConnector {
	return &mailConnector{
		service: service,
		log:     logger.New("test"),
		updates: make(chan imap.Update, updateBufferSize),
	}
}

// TestGetMessageLiteralServesUndecryptedBody is the behaviour that keeps one
// unreadable message from costing a client the whole fetch: an email sealed for
// another address still reaches it, carrying its envelope.
func TestGetMessageLiteralServesUndecryptedBody(t *testing.T) {
	body := []byte("Subject: cifrado\r\n\r\nINTERNXT-ENCRYPTED-EMAIL-v1\r\n")
	connector := testConnector(&fakeMailService{
		literal: body,
		err:     errors.New("no wrapped key for this address"),
	})

	literal, err := connector.GetMessageLiteral(context.Background(), "M1")
	if err != nil {
		t.Fatalf("GetMessageLiteral: %v", err)
	}
	if string(literal) != string(body) {
		t.Fatalf("got %q, want the message as it arrived", literal)
	}
}

// TestGetMessageLiteralFailsWithoutAMessage covers the other half: nothing to
// serve is a real failure, and passing it off as an empty message would look
// like a genuine blank email.
func TestGetMessageLiteralFailsWithoutAMessage(t *testing.T) {
	connector := testConnector(&fakeMailService{err: errors.New("email not found")})

	if _, err := connector.GetMessageLiteral(context.Background(), "M1"); err == nil {
		t.Fatal("expected an error when there is no message to serve")
	}
}

// TestSyncForgetsThreadsAfterwards keeps the sync-scoped cache from outliving
// the sync: held any longer, it would serve a client mail that has since
// changed.
func TestSyncForgetsThreadsAfterwards(t *testing.T) {
	service := &fakeMailService{}
	connector := testConnector(service)

	if err := connector.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if service.forgotten != 1 {
		t.Fatalf("ForgetThreads called %d times, want 1", service.forgotten)
	}
}

func TestGetMessageLiteralReturnsTheBody(t *testing.T) {
	body := []byte("Subject: claro\r\n\r\nhola\r\n")
	connector := testConnector(&fakeMailService{literal: body})

	literal, err := connector.GetMessageLiteral(context.Background(), "M1")
	if err != nil {
		t.Fatalf("GetMessageLiteral: %v", err)
	}
	if string(literal) != string(body) {
		t.Fatalf("got %q, want %q", literal, body)
	}
}
