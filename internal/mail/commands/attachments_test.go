package commands

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"mail-bridge-desktop/internal/api"
	"mail-bridge-desktop/internal/crypto"
)

// sealedAttachment encrypts content under the session key the fixture envelope
// carries, which is how an attachment really travels: one key per email,
// shared by every attachment in it.
func sealedAttachment(t *testing.T, email api.EmailResponseDto, account Account, content []byte) []byte {
	t.Helper()

	sessionKey, err := attachmentsSessionKey(email, account)
	if err != nil {
		t.Fatalf("attachmentsSessionKey: %v", err)
	}
	if len(sessionKey) == 0 {
		t.Fatal("the fixture envelope carries no attachments session key")
	}

	sealed, err := crypto.EncryptSymmetrically(sessionKey, content, nil)
	if err != nil {
		t.Fatalf("EncryptSymmetrically: %v", err)
	}
	return sealed
}

func withAttachment(email api.EmailResponseDto, blobID, name string) api.EmailResponseDto {
	email.Attachments = []api.EmailAttachmentDto{{BlobId: blobID, Name: name, Type: "text/plain"}}
	email.HasAttachment = true
	return email
}

func TestDownloadAttachmentsDecryptsThem(t *testing.T) {
	account := testAccount(t, testAddress, testPrivateKey)
	email := withAttachment(encryptedEmail(t), "B1", "notas.txt")
	content := []byte("el contenido del adjunto")

	client := &fakeClient{blobs: map[string][]byte{"B1": sealedAttachment(t, email, account, content)}}

	blobs, err := DownloadAttachments(context.Background(), client, "tok", email, account, func(err error) {
		t.Errorf("unexpected failure: %v", err)
	})
	if err != nil {
		t.Fatalf("DownloadAttachments: %v", err)
	}

	if !bytes.Equal(blobs["B1"], content) {
		t.Errorf("attachment = %q, want %q", blobs["B1"], content)
	}
}

// TestDownloadAttachmentsWithoutAttachmentsAsksForNothing keeps the ordinary
// email off the network: most mail has no attachments, and this runs on every
// fetch.
func TestDownloadAttachmentsWithoutAttachmentsAsksForNothing(t *testing.T) {
	client := &fakeClient{}
	account := testAccount(t, testAddress, testPrivateKey)

	blobs, err := DownloadAttachments(context.Background(), client, "tok", encryptedEmail(t), account, func(err error) {
		t.Errorf("unexpected failure: %v", err)
	})
	if err != nil {
		t.Fatalf("DownloadAttachments: %v", err)
	}
	if len(blobs) != 0 {
		t.Errorf("got %d blobs, want none", len(blobs))
	}
	if len(client.downloadedBlobs) != 0 {
		t.Errorf("downloaded %v, want nothing", client.downloadedBlobs)
	}
}

// TestDownloadAttachmentsSurvivesAFailedOne is what keeps one unreadable
// attachment from costing the client the whole email, mirroring how an
// unreadable body is already handled.
func TestDownloadAttachmentsSurvivesAFailedOne(t *testing.T) {
	account := testAccount(t, testAddress, testPrivateKey)
	email := withAttachment(encryptedEmail(t), "B1", "notas.txt")

	client := &fakeClient{downloadErr: errors.New("the blob is gone")}

	var reported []error
	blobs, err := DownloadAttachments(context.Background(), client, "tok", email, account, func(err error) {
		reported = append(reported, err)
	})
	if err != nil {
		t.Fatalf("DownloadAttachments: %v", err)
	}
	if len(blobs) != 0 {
		t.Errorf("got %d blobs, want none to have been served", len(blobs))
	}
	if len(reported) != 1 {
		t.Errorf("reported %d failures, want 1", len(reported))
	}
}

// TestDownloadAttachmentsOfAPlainEmailServesThemAsThey Are covers mail that was
// never encrypted: there is no session key, so the bytes stand as downloaded.
func TestDownloadAttachmentsOfAPlainEmailServesThemAsTheyAre(t *testing.T) {
	body := "un correo sin cifrar"
	email := withAttachment(api.EmailResponseDto{Id: "M1", TextBody: &body}, "B1", "notas.txt")
	content := []byte("contenido en claro")

	client := &fakeClient{blobs: map[string][]byte{"B1": content}}

	blobs, err := DownloadAttachments(context.Background(), client, "tok", email, Account{}, func(err error) {
		t.Errorf("unexpected failure: %v", err)
	})
	if err != nil {
		t.Fatalf("DownloadAttachments: %v", err)
	}
	if !bytes.Equal(blobs["B1"], content) {
		t.Errorf("attachment = %q, want the bytes as downloaded", blobs["B1"])
	}
}

// TestDownloadAttachmentsWithoutKeysFails refuses to guess: an encrypted email
// whose keys are missing cannot have its attachments served, and saying so is
// better than handing the client ciphertext named like a document.
func TestDownloadAttachmentsWithoutKeysFails(t *testing.T) {
	email := withAttachment(encryptedEmail(t), "B1", "notas.txt")
	client := &fakeClient{}

	if _, err := DownloadAttachments(context.Background(), client, "tok", email, Account{}, func(error) {}); err == nil {
		t.Fatal("expected an error when the account holds no keys")
	}
}
