package mailconnector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ProtonMail/gluon/connector"
	"github.com/ProtonMail/gluon/imap"

	"mail-bridge-desktop/internal/api"
)

// inboxID and trashID are the IDs the API gives those folders; the connector
// learns which is which while syncing.
const (
	inboxID  = imap.MailboxID("a")
	trashID  = imap.MailboxID("b")
	draftsID = imap.MailboxID("c")
)

func connectorWithMailboxes(service MailService) *MailConnector {
	c := testConnector(service)
	c.rememberMailboxType(api.MailboxResponseDto{Id: string(inboxID), Type: mailboxTypePtr(api.MailboxInbox)})
	c.rememberMailboxType(api.MailboxResponseDto{Id: string(trashID), Type: mailboxTypePtr(api.MailboxTrash)})
	c.rememberMailboxType(api.MailboxResponseDto{Id: string(draftsID), Type: mailboxTypePtr(api.MailboxDrafts)})
	return c
}

func mailboxTypePtr(kind api.Mailbox) *api.Mailbox {
	return &kind
}

func TestMarkMessagesSeenReachesTheService(t *testing.T) {
	service := &fakeMailService{}
	c := connectorWithMailboxes(service)

	if err := c.MarkMessagesSeen(context.Background(), []imap.MessageID{"M1", "M2"}, true); err != nil {
		t.Fatalf("MarkMessagesSeen: %v", err)
	}
	if len(service.markedRead) != 2 || service.markedRead[0] != "M1" {
		t.Fatalf("marked %v, want [M1 M2]", service.markedRead)
	}
	if !service.readValue {
		t.Error("the messages should have been marked read")
	}
}

func TestMarkMessagesFlaggedReachesTheService(t *testing.T) {
	service := &fakeMailService{}
	c := connectorWithMailboxes(service)

	if err := c.MarkMessagesFlagged(context.Background(), []imap.MessageID{"M1"}, true); err != nil {
		t.Fatalf("MarkMessagesFlagged: %v", err)
	}
	if len(service.markedFlagged) != 1 {
		t.Fatalf("flagged %v, want [M1]", service.markedFlagged)
	}
}

// TestMoveMessagesNamesTheDestination covers the translation the API needs: it
// moves mail by the kind of folder, while Gluon speaks in the IDs the listing
// gave out.
func TestMoveMessagesNamesTheDestination(t *testing.T) {
	service := &fakeMailService{}
	c := connectorWithMailboxes(service)

	removed, err := c.MoveMessages(context.Background(), []imap.MessageID{"M1"}, inboxID, trashID)
	if err != nil {
		t.Fatalf("MoveMessages: %v", err)
	}
	if !removed {
		t.Error("a move takes the message out of the source folder")
	}
	if service.movedTo != api.MailboxTrash {
		t.Fatalf("moved to %q, want trash", service.movedTo)
	}
}

// TestMoveMessagesRefusesAnUnknownMailbox keeps a bad destination from being
// sent to the API, which would reject it with a worse message.
func TestMoveMessagesRefusesAnUnknownMailbox(t *testing.T) {
	service := &fakeMailService{}
	c := connectorWithMailboxes(service)

	if _, err := c.MoveMessages(context.Background(), []imap.MessageID{"M1"}, inboxID, "zzz"); err == nil {
		t.Fatal("expected an error for a mailbox the sync never saw")
	}
	if service.moved != nil {
		t.Error("nothing should have been moved")
	}
}

// TestRemoveMessagesFromTrashDeletes is the distinction that makes emptying the
// trash mean what it says, while removing from any other folder is a move.
func TestRemoveMessagesFromTrashDeletes(t *testing.T) {
	service := &fakeMailService{}
	c := connectorWithMailboxes(service)

	if err := c.RemoveMessagesFromMailbox(context.Background(), []imap.MessageID{"M1"}, trashID); err != nil {
		t.Fatalf("RemoveMessagesFromMailbox: %v", err)
	}
	if len(service.deleted) != 1 {
		t.Fatalf("deleted %v, want [M1]", service.deleted)
	}
	if service.moved != nil {
		t.Error("mail in the trash is deleted, not moved")
	}
}

func TestRemoveMessagesFromInboxMovesToTrash(t *testing.T) {
	service := &fakeMailService{}
	c := connectorWithMailboxes(service)

	if err := c.RemoveMessagesFromMailbox(context.Background(), []imap.MessageID{"M1"}, inboxID); err != nil {
		t.Fatalf("RemoveMessagesFromMailbox: %v", err)
	}
	if service.movedTo != api.MailboxTrash {
		t.Fatalf("moved to %q, want trash", service.movedTo)
	}
	if service.deleted != nil {
		t.Error("mail outside the trash should be moved, not deleted")
	}
}

// TestMutationsReportFailure keeps a failed API call from looking like success:
// a client told the change went through would show state the account does not
// have.
func TestMutationsReportFailure(t *testing.T) {
	service := &fakeMailService{writeErr: errors.New("api is down")}
	c := connectorWithMailboxes(service)

	if err := c.MarkMessagesSeen(context.Background(), []imap.MessageID{"M1"}, true); err == nil {
		t.Error("MarkMessagesSeen should report the failure")
	}
	if _, err := c.MoveMessages(context.Background(), []imap.MessageID{"M1"}, inboxID, trashID); err == nil {
		t.Error("MoveMessages should report the failure")
	}
}

// TestCreateMessageInDraftsSavesADraft is the append a client makes every time
// it autosaves what is being written.
func TestCreateMessageInDraftsSavesADraft(t *testing.T) {
	service := &fakeMailService{draftID: "D7"}
	c := connectorWithMailboxes(service)

	literal := []byte("Subject: a medias\r\n\r\nlo termino luego\r\n")
	flags := imap.NewFlagSet(imap.FlagSeen, imap.FlagDraft)
	date := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)

	message, returned, err := c.CreateMessage(context.Background(), draftsID, literal, flags, date)
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	if string(service.savedDraft) != string(literal) {
		t.Errorf("saved %q, want the literal as it arrived", service.savedDraft)
	}
	// The ID has to be the one the API just assigned: Gluon rejects an append
	// to drafts that comes back with a remote ID it already knows.
	if message.ID != imap.MessageID("D7") {
		t.Errorf("message id = %q, want D7", message.ID)
	}
	if !message.Flags.Contains(imap.FlagDraft) {
		t.Error("the flags the client sent should come back")
	}
	if !message.Date.Equal(date) {
		t.Errorf("date = %v, want %v", message.Date, date)
	}
	if string(returned) != string(literal) {
		t.Error("the literal should go back untouched")
	}
}

// TestCreateMessageOutsideDraftsIsRefused covers the append a client makes to
// Sent after delivering: the API has no way to file a message into a folder,
// so it is refused rather than silently dropped.
func TestCreateMessageOutsideDraftsIsRefused(t *testing.T) {
	service := &fakeMailService{}
	c := connectorWithMailboxes(service)

	_, _, err := c.CreateMessage(context.Background(), inboxID, []byte("x"), imap.NewFlagSet(), time.Now())
	if !errors.Is(err, connector.ErrOperationNotAllowed) {
		t.Errorf("CreateMessage: got %v, want ErrOperationNotAllowed", err)
	}
	if service.savedDraft != nil {
		t.Error("nothing should have been saved")
	}
}

func TestCreateMessageReportsAFailedSave(t *testing.T) {
	service := &fakeMailService{writeErr: errors.New("api is down")}
	c := connectorWithMailboxes(service)

	if _, _, err := c.CreateMessage(context.Background(), draftsID, []byte("x"), imap.NewFlagSet(), time.Now()); err == nil {
		t.Fatal("expected the failure to reach the client")
	}
}

// TestFolderOperationsAreRefused documents what the API does not offer: it
// lists folders and gives no way to change them.
func TestFolderOperationsAreRefused(t *testing.T) {
	c := connectorWithMailboxes(&fakeMailService{})
	ctx := context.Background()

	if _, err := c.CreateMailbox(ctx, []string{"Nueva"}); !errors.Is(err, connector.ErrOperationNotAllowed) {
		t.Errorf("CreateMailbox: got %v", err)
	}
	if err := c.UpdateMailboxName(ctx, inboxID, []string{"Otra"}); !errors.Is(err, connector.ErrOperationNotAllowed) {
		t.Errorf("UpdateMailboxName: got %v", err)
	}
	if err := c.DeleteMailbox(ctx, inboxID); !errors.Is(err, connector.ErrOperationNotAllowed) {
		t.Errorf("DeleteMailbox: got %v", err)
	}
}
