package mail

import (
	"fmt"
	"strings"
	"testing"
)

// SendEmail files what it sent through rememberSentCopy, which is the one line that
// wires these together; what is worth pinning is how a copy is found again.
const appleMailMessage = "Message-Id: <A1B2C3@smtpclient.apple>\r\n" +
	"From: someone@inxt.me\r\n" +
	"To: bob@example.test\r\n" +
	"Subject: hello\r\n" +
	"\r\n" +
	"body\r\n"

func testSentCopies(t *testing.T) *MailService {
	t.Helper()
	return testService(t, &fakeClient{}, "")
}

// TestSentCopyIsFoundByItsMessageID is what keeps a client's copy of a sent message and
// the account's from becoming two. The client appends its own copy to Sent seconds
// after sending, and the Message-ID it stamped is the only thing the two have in
// common — so that is what the copy is filed under.
func TestSentCopyIsFoundByItsMessageID(t *testing.T) {
	service := testSentCopies(t)
	service.rememberSentCopy([]byte(appleMailMessage), "M1a2b3c")

	id, sent := service.SentCopyID([]byte(appleMailMessage))
	if !sent {
		t.Fatal("the message just sent is not recognised, so its copy would be stored twice")
	}
	if id != "M1a2b3c" {
		t.Errorf("SentCopyID = %q, want the ID the backend gave the copy it stored", id)
	}
}

// TestSentCopyIgnoresAMessageThisBridgeNeverSent covers a client filing something into
// Sent on its own. There is no copy in the account to point at.
func TestSentCopyIgnoresAMessageThisBridgeNeverSent(t *testing.T) {
	service := testSentCopies(t)
	service.rememberSentCopy(messageWithID("<sent@smtpclient.apple>"), "M1a2b3c")

	if _, sent := service.SentCopyID(messageWithID("<other@smtpclient.apple>")); sent {
		t.Error("a message that was never sent through the bridge has no stored copy")
	}
}

// TestSentCopyNeedsAMessageID covers a client that stamps none: there would be nothing
// to tell one message from another, and answering with the wrong copy is worse than
// refusing the append.
func TestSentCopyNeedsAMessageID(t *testing.T) {
	service := testSentCopies(t)
	withoutID := []byte("From: someone@inxt.me\r\nSubject: hello\r\n\r\nbody\r\n")

	service.rememberSentCopy(withoutID, "M1a2b3c")
	if _, sent := service.SentCopyID(withoutID); sent {
		t.Error("a message with no Message-ID was filed, so any other such message would match it")
	}
}

// TestSentCopiesForgetTheOldest bounds what a long-running bridge keeps: a copy is
// asked for seconds after it is filed, so only the last few are ever of use.
func TestSentCopiesForgetTheOldest(t *testing.T) {
	service := testSentCopies(t)

	first := messageWithID("<first@smtpclient.apple>")
	service.rememberSentCopy(first, "M1a2b3c")

	for i := 0; i < sentCopiesKept; i++ {
		service.rememberSentCopy(messageWithID(fmt.Sprintf("<later-%d@smtpclient.apple>", i)), "M1a2b3c")
	}

	if _, sent := service.SentCopyID(first); sent {
		t.Error("the oldest copy is still remembered; the map would grow for as long as the bridge runs")
	}
	if len(service.sentCopies) > sentCopiesKept {
		t.Errorf("kept %d copies, want at most %d", len(service.sentCopies), sentCopiesKept)
	}
}

func messageWithID(messageID string) []byte {
	return []byte(strings.Replace(appleMailMessage, "<A1B2C3@smtpclient.apple>", messageID, 1))
}
