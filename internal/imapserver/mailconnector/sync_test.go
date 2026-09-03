package mailconnector

import (
	"context"
	"errors"
	"testing"

	"github.com/ProtonMail/gluon/imap"

	"mail-bridge-desktop/internal/api"
)

func inboxMailbox() api.MailboxResponseDto {
	kind := api.MailboxInbox
	return api.MailboxResponseDto{Id: string(inboxID), Name: "Inbox", Type: &kind}
}

func sentMailbox() api.MailboxResponseDto {
	kind := api.MailboxSent
	return api.MailboxResponseDto{Id: "e", Name: "Sent Items", Type: &kind}
}

func summary(id string, mailboxIDs ...string) api.EmailSummaryResponseDto {
	return api.EmailSummaryResponseDto{
		Id:         id,
		Subject:    "asunto " + id,
		ReceivedAt: "2026-07-24T10:00:00Z",
		From:       []api.EmailAddressDto{{Email: "someone@example.test"}},
		MailboxIds: mailboxIDs,
	}
}

// syncService is a fake holding one inbox, ready for a sync.
func syncService(summaries ...api.EmailSummaryResponseDto) *fakeMailService {
	return &fakeMailService{
		literal:   []byte("Subject: asunto\r\n\r\ncuerpo\r\n"),
		mailboxes: []api.MailboxResponseDto{inboxMailbox()},
		summaries: map[api.Mailbox][]api.EmailSummaryResponseDto{
			api.MailboxInbox: summaries,
		},
	}
}

// drainUpdates collects what the sync announced, so a test can assert on it.
func drainUpdates(c *MailConnector) []imap.Update {
	var updates []imap.Update
	for {
		select {
		case update := <-c.updates:
			updates = append(updates, update)
		default:
			return updates
		}
	}
}

func countUpdates[T imap.Update](updates []imap.Update) int {
	var found int
	for _, update := range updates {
		if _, ok := update.(T); ok {
			found++
		}
	}
	return found
}

// TestSyncAnnouncesNewMail is the first sync: everything is new, so every
// message is created and every body fetched.
func TestSyncAnnouncesNewMail(t *testing.T) {
	service := syncService(summary("M1", "a"), summary("M2", "a"))
	c := testConnector(service)

	if err := c.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	updates := drainUpdates(c)
	if got := countUpdates[*imap.MessagesCreated](updates); got != 1 {
		t.Fatalf("got %d MessagesCreated, want 1", got)
	}
	if got := len(service.fetchedBodies()); got != 2 {
		t.Fatalf("fetched %d bodies, want 2", got)
	}
}

// TestSyncIsQuietWhenNothingChanged is what makes polling cheap: a second run
// over the same mailbox announces nothing and downloads nothing.
func TestSyncIsQuietWhenNothingChanged(t *testing.T) {
	service := syncService(summary("M1", "a"), summary("M2", "a"))
	c := testConnector(service)
	ctx := context.Background()

	if err := c.Sync(ctx); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	drainUpdates(c)
	fetchedFirst := len(service.fetchedBodies())

	if err := c.Sync(ctx); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	if updates := drainUpdates(c); len(updates) != 0 {
		t.Fatalf("second sync announced %d updates, want none", len(updates))
	}
	if got := len(service.fetchedBodies()); got != fetchedFirst {
		t.Fatalf("second sync fetched %d more bodies, want none", got-fetchedFirst)
	}
}

// TestSyncReportsMailReadElsewhere covers a change made in the web client: the
// flag is updated without the body being fetched again.
func TestSyncReportsMailReadElsewhere(t *testing.T) {
	service := syncService(summary("M1", "a"))
	c := testConnector(service)
	ctx := context.Background()

	if err := c.Sync(ctx); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	drainUpdates(c)
	fetchedFirst := len(service.fetchedBodies())

	read := summary("M1", "a")
	read.IsRead = true
	service.summaries[api.MailboxInbox] = []api.EmailSummaryResponseDto{read}

	if err := c.Sync(ctx); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	updates := drainUpdates(c)
	if got := countUpdates[*imap.MessageMailboxesUpdated](updates); got != 1 {
		t.Fatalf("got %d MessageMailboxesUpdated, want 1", got)
	}
	if got := len(service.fetchedBodies()); got != fetchedFirst {
		t.Fatal("a flag change should not fetch the body again")
	}
}

// TestSyncRemovesMailThatIsGone covers mail deleted elsewhere.
func TestSyncRemovesMailThatIsGone(t *testing.T) {
	service := syncService(summary("M1", "a"), summary("M2", "a"))
	c := testConnector(service)
	ctx := context.Background()

	if err := c.Sync(ctx); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	drainUpdates(c)

	service.summaries[api.MailboxInbox] = []api.EmailSummaryResponseDto{summary("M1", "a")}

	if err := c.Sync(ctx); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	updates := drainUpdates(c)
	if got := countUpdates[*imap.MessageDeleted](updates); got != 1 {
		t.Fatalf("got %d MessageDeleted, want 1", got)
	}
}

// TestSyncKeepsMailWhenAFolderFails is the guard against deleting mail over a
// failed request: a folder that did not answer leaves its messages unseen, and
// treating that as gone would remove mail the account still has.
func TestSyncKeepsMailWhenAFolderFails(t *testing.T) {
	service := syncService(summary("M1", "a"))
	service.mailboxes = append(service.mailboxes, sentMailbox())
	service.summaries[api.MailboxSent] = []api.EmailSummaryResponseDto{summary("M2", "e")}

	c := testConnector(service)
	ctx := context.Background()

	if err := c.Sync(ctx); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	drainUpdates(c)

	// The sent folder stops answering, so its messages go unseen this cycle.
	service.listErr = map[api.Mailbox]error{api.MailboxSent: errors.New("api is down")}

	if err := c.Sync(ctx); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	updates := drainUpdates(c)
	if got := countUpdates[*imap.MessageDeleted](updates); got != 0 {
		t.Fatalf("a partial sync deleted %d messages, want none", got)
	}
}

// TestSyncAnnouncesAFolderOnce keeps a poll from repeating the mailbox
// announcement every cycle for no reason.
func TestSyncAnnouncesAFolderOnce(t *testing.T) {
	service := syncService(summary("M1", "a"))
	c := testConnector(service)
	ctx := context.Background()

	if err := c.Sync(ctx); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	first := countUpdates[*imap.MailboxCreated](drainUpdates(c))

	if err := c.Sync(ctx); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	second := countUpdates[*imap.MailboxCreated](drainUpdates(c))

	if first != 1 || second != 0 {
		t.Fatalf("announced the mailbox %d then %d times, want 1 then 0", first, second)
	}
}
