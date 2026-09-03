package mailconnector

import (
	"context"
	"errors"
	"testing"

	"github.com/ProtonMail/gluon/connector"
	"github.com/ProtonMail/gluon/imap"

	"mail-bridge-desktop/internal/api"
)

// inboxID and trashID are the IDs the API gives those folders; the connector
// learns which is which while syncing.
const (
	inboxID = imap.MailboxID("a")
	trashID = imap.MailboxID("b")
)

func connectorWithMailboxes(service MailService) *MailConnector {
	c := testConnector(service)
	c.rememberMailboxType(api.MailboxResponseDto{Id: string(inboxID), Type: mailboxTypePtr(api.MailboxInbox)})
	c.rememberMailboxType(api.MailboxResponseDto{Id: string(trashID), Type: mailboxTypePtr(api.MailboxTrash)})
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
