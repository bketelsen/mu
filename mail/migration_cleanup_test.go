package mail

import (
	"testing"

	"mu/internal/data"
)

func TestDeleteInboxHydratesPersistedMessagesBeforeLoad(t *testing.T) {
	if err := data.SaveJSON("mail.json", []*Message{
		{ID: "deleted-message", FromID: "deleted", ToID: "survivor", Subject: "remove"},
		{ID: "survivor-message", FromID: "survivor", ToID: "external", Subject: "keep"},
	}); err != nil {
		t.Fatal(err)
	}

	mutex.Lock()
	oldMessages, oldInboxes := messages, inboxes
	messages, inboxes = nil, nil
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		messages, inboxes = oldMessages, oldInboxes
		mutex.Unlock()
	})

	if err := DeleteInbox("deleted"); err != nil {
		t.Fatal(err)
	}

	var got []*Message
	if err := data.LoadJSON("mail.json", &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "survivor-message" {
		t.Fatalf("messages after pre-load cleanup = %#v, want only survivor message", got)
	}
}
