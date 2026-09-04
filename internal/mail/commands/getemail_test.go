package commands

import (
	"context"
	"errors"
	"testing"

	"mail-bridge-desktop/internal/api"
)

type fakeClient struct {
	thread []api.EmailResponseDto
	err    error

	// What the write commands asked for, so a test can check the call rather
	// than only its result.
	updated    []string
	lastUpdate api.UpdateEmailRequestDto
	deleted    []string

	// What SendEmail's collaborators return, and what it asked of them.
	recipientKeys      []api.RecipientKeyDto
	lookupErr          error
	sendErr            error
	sentEmail          api.SendEmailRequestDto
	sendCalled         bool
	mailAccountKeys    api.MailAccountKeysResponseDto
	mailAccountKeysErr error
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

func (f *fakeClient) UpdateEmail(ctx context.Context, token, emailID string, update api.UpdateEmailRequestDto) error {
	f.updated = append(f.updated, emailID)
	f.lastUpdate = update
	return f.err
}

func (f *fakeClient) DeleteEmail(ctx context.Context, token, emailID string) error {
	f.deleted = append(f.deleted, emailID)
	return f.err
}

func (f *fakeClient) LookupRecipientKeys(ctx context.Context, token string, addresses []string) ([]api.RecipientKeyDto, error) {
	return f.recipientKeys, f.lookupErr
}

func (f *fakeClient) SendEmail(ctx context.Context, token string, email api.SendEmailRequestDto) (api.EmailCreatedResponseDto, error) {
	f.sendCalled = true
	f.sentEmail = email
	if f.sendErr != nil {
		return api.EmailCreatedResponseDto{}, f.sendErr
	}
	return api.EmailCreatedResponseDto{Id: "M1"}, nil
}

func (f *fakeClient) GetMailAccountKeys(ctx context.Context, token string) (api.MailAccountKeysResponseDto, error) {
	return f.mailAccountKeys, f.mailAccountKeysErr
}

func TestGetEmailPicksItOutOfTheThread(t *testing.T) {
	client := &fakeClient{thread: []api.EmailResponseDto{
		{Id: "M1", Subject: "primero"},
		{Id: "M2", Subject: "segundo"},
	}}

	email, err := GetEmail(context.Background(), client, "tok", "M2", Account{})
	if err != nil {
		t.Fatalf("GetEmail: %v", err)
	}
	if email.Subject != "segundo" {
		t.Fatalf("got %q, want segundo", email.Subject)
	}
}

// A message in two folders has one ID per copy, and the thread comes back
// carrying only one of them.
func TestGetEmailAcceptsSingleMessageThreadWithAnotherID(t *testing.T) {
	client := &fakeClient{thread: []api.EmailResponseDto{
		{Id: "jyaaaacp", Subject: "la otra copia"},
	}}

	email, err := GetEmail(context.Background(), client, "tok", "jyaaaaco", Account{})
	if err != nil {
		t.Fatalf("GetEmail: %v", err)
	}
	if email.Subject != "la otra copia" {
		t.Fatalf("got %q", email.Subject)
	}
}

// With several messages and no ID match there is no way to tell which one was
// meant, so it is an error rather than a guess.
func TestGetEmailNotFoundInThread(t *testing.T) {
	client := &fakeClient{thread: []api.EmailResponseDto{
		{Id: "M1"},
		{Id: "M2"},
	}}

	if _, err := GetEmail(context.Background(), client, "tok", "M9", Account{}); !errors.Is(err, ErrEmailNotFound) {
		t.Fatalf("errors.Is(err, ErrEmailNotFound) = false, err = %v", err)
	}
}

func TestGetEmailPropagatesClientError(t *testing.T) {
	client := &fakeClient{err: errors.New("boom")}

	if _, err := GetEmail(context.Background(), client, "tok", "M1", Account{}); err == nil {
		t.Fatal("expected an error")
	}
}
