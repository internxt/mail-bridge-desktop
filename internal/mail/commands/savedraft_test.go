package commands

import (
	"context"
	"errors"
	"testing"

	"mail-bridge-desktop/internal/api"
)

func draftAccount(t *testing.T) Account {
	t.Helper()
	return Account{Address: "alice@inxt.eu", PublicKey: testRecipientPublicKey(t)}
}

// TestSaveDraftSealsForTheSenderOnly is the point of the whole command: a
// draft is not delivered, so nobody but its author should be able to open it,
// not even a recipient already named in the To: line.
func TestSaveDraftSealsForTheSenderOnly(t *testing.T) {
	client := &fakeClient{}

	draft, err := SaveDraft(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "a medias",
		TextBody: "lo termino luego",
		To:       []api.EmailAddressDto{addr("bob@inxt.eu")},
	}, draftAccount(t))
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	if draft.Id != "D1" {
		t.Errorf("draft id = %q, want D1", draft.Id)
	}

	saved := client.savedDraft
	if saved.Encryption == nil {
		t.Fatal("expected the draft to carry an encryption block")
	}
	if len(saved.Encryption.WrappedKeys) != 1 {
		t.Fatalf("wrapped keys = %d, want 1 (the sender alone)", len(saved.Encryption.WrappedKeys))
	}
	if got := saved.Encryption.WrappedKeys[0].EncryptedForEmail; got != "alice@inxt.eu" {
		t.Errorf("wrapped for %q, want alice@inxt.eu (the sender)", got)
	}

	body := sealedBody(t, saved.Encryption, testRecipientPrivateKeyHex, "alice@inxt.eu")
	if body.Text != "lo termino luego" {
		t.Errorf("sealed body = %q, want the draft body", body.Text)
	}
}

// TestSaveDraftSealsTheHTMLBody mirrors the send path: the envelope holds one
// body, and it is read back as HTML.
func TestSaveDraftSealsTheHTMLBody(t *testing.T) {
	client := &fakeClient{}

	_, err := SaveDraft(context.Background(), client, "tok", OutgoingMessage{
		Subject:  "a medias",
		TextBody: "texto plano",
		HTMLBody: "<p>en HTML</p>",
	}, draftAccount(t))
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	body := sealedBody(t, client.savedDraft.Encryption, testRecipientPrivateKeyHex, "alice@inxt.eu")
	if body.Text != "<p>en HTML</p>" {
		t.Errorf("sealed body = %q, want the HTML body", body.Text)
	}
}

// TestSaveDraftSendsRecipientsAsMetadata keeps the addresses travelling in the
// clear next to the sealed body, so a listing can show who the draft is for.
func TestSaveDraftSendsRecipientsAsMetadata(t *testing.T) {
	client := &fakeClient{}

	_, err := SaveDraft(context.Background(), client, "tok", OutgoingMessage{
		Subject: "a medias",
		To:      []api.EmailAddressDto{addr("bob@inxt.eu")},
		Cc:      []api.EmailAddressDto{addr("carol@inxt.eu")},
	}, draftAccount(t))
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	saved := client.savedDraft
	if saved.To == nil || len(*saved.To) != 1 || (*saved.To)[0].Email != "bob@inxt.eu" {
		t.Errorf("to = %v, want [bob@inxt.eu]", saved.To)
	}
	if saved.Cc == nil || len(*saved.Cc) != 1 {
		t.Errorf("cc = %v, want [carol@inxt.eu]", saved.Cc)
	}
	if saved.Bcc != nil {
		t.Errorf("bcc = %v, want nil when there is none", saved.Bcc)
	}
	if saved.Subject == nil || *saved.Subject != "a medias" {
		t.Errorf("subject = %v, want a medias", saved.Subject)
	}
}

// TestSaveDraftOmitsAnEmptySubject covers the half-written draft: every field
// of the DTO is optional, so a blank subject is left out rather than sent as
// an empty string.
func TestSaveDraftOmitsAnEmptySubject(t *testing.T) {
	client := &fakeClient{}

	if _, err := SaveDraft(context.Background(), client, "tok", OutgoingMessage{
		TextBody: "sin asunto todavía",
	}, draftAccount(t)); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	if client.savedDraft.Subject != nil {
		t.Errorf("subject = %v, want nil", client.savedDraft.Subject)
	}
}

// TestSaveDraftWithoutAccountKeyFails refuses to store a draft in the clear
// when the account key never arrived.
func TestSaveDraftWithoutAccountKeyFails(t *testing.T) {
	client := &fakeClient{}

	_, err := SaveDraft(context.Background(), client, "tok", OutgoingMessage{
		Subject: "a medias",
	}, Account{Address: "alice@inxt.eu"})
	if err == nil {
		t.Fatal("expected an error: no account key to seal the draft with")
	}
	if client.saveDraftCalled {
		t.Error("SaveDraft should not have reached the client")
	}
}

func TestSaveDraftPropagatesClientFailure(t *testing.T) {
	client := &fakeClient{saveDraftErr: errors.New("api is down")}

	if _, err := SaveDraft(context.Background(), client, "tok", OutgoingMessage{
		Subject: "a medias",
	}, draftAccount(t)); err == nil {
		t.Fatal("expected the client failure to propagate")
	}
}
