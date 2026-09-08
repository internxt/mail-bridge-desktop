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
	sentID   = imap.MailboxID("d")
)

func connectorWithMailboxes(service MailService) *MailConnector {
	c := testConnector(service)
	c.rememberMailboxType(api.MailboxResponseDto{Id: string(inboxID), Type: mailboxTypePtr(api.MailboxInbox)})
	c.rememberMailboxType(api.MailboxResponseDto{Id: string(trashID), Type: mailboxTypePtr(api.MailboxTrash)})
	c.rememberMailboxType(api.MailboxResponseDto{Id: string(draftsID), Type: mailboxTypePtr(api.MailboxDrafts)})
	c.rememberMailboxType(api.MailboxResponseDto{Id: string(sentID), Type: mailboxTypePtr(api.MailboxSent)})
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

func TestRemoveMessagesFromDraftsDiscardsThem(t *testing.T) {
	service := &fakeMailService{}
	c := connectorWithMailboxes(service)

	if err := c.RemoveMessagesFromMailbox(context.Background(), []imap.MessageID{"D1"}, draftsID); err != nil {
		t.Fatalf("RemoveMessagesFromMailbox: %v", err)
	}
	if len(service.discardedDrafts) != 1 || service.discardedDrafts[0] != "D1" {
		t.Fatalf("discarded %v, want [D1]", service.discardedDrafts)
	}
	if service.moved != nil {
		t.Error("a replaced draft should not be moved to the trash")
	}
	if service.deleted != nil {
		t.Error("a draft is discarded through its own endpoint, not deleted as ordinary mail")
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

// TestCreateMessageOutsideDraftsIsRefused covers a folder the API cannot file
// a message into.
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

// TestCreateMessageInSentIsAcceptedAndDropped covers the copy a client files
// after delivering. The backend already stored one while sending, so keeping
// this would show the message twice — and refusing it makes the client report
// that it could not save the copy.
func TestCreateMessageInSentIsAcceptedAndDropped(t *testing.T) {
	service := &fakeMailService{}
	c := connectorWithMailboxes(service)

	message, _, err := c.CreateMessage(context.Background(), sentID, []byte("x"), imap.NewFlagSet(), time.Now())
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if service.savedDraft != nil {
		t.Error("a sent copy should not be stored as a draft")
	}

	// It has to be taken back: a sync never reports it as deleted, since it was
	// never in the account to begin with.
	select {
	case update := <-c.updates:
		deleted, isDeletion := update.(*imap.MessageDeleted)
		if !isDeletion {
			t.Fatalf("update = %T, want a deletion", update)
		}
		if deleted.MessageID != message.ID {
			t.Errorf("deleted %v, want the message just accepted (%v)", deleted.MessageID, message.ID)
		}
	default:
		t.Fatal("the accepted copy was never taken back, so it would linger in the client")
	}
}

// TestCreateMessageInSentIsUniqueEachTime guards the rule Gluon enforces on an
// append: a remote ID it already knows is an error.
func TestCreateMessageInSentIsUniqueEachTime(t *testing.T) {
	c := connectorWithMailboxes(&fakeMailService{})

	first, _, err := c.CreateMessage(context.Background(), sentID, []byte("x"), imap.NewFlagSet(), time.Now())
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	second, _, err := c.CreateMessage(context.Background(), sentID, []byte("x"), imap.NewFlagSet(), time.Now())
	if err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	if first.ID == second.ID {
		t.Errorf("both appends got id %q; Gluon rejects a repeated remote ID", first.ID)
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
