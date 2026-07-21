package mail

import (
	"testing"
	"time"

	"mu/internal/auth"
)

func TestSMTPAcceptsOnlyOwnerLocalRecipient(t *testing.T) {
	owner, err := auth.Owner()
	if err != nil {
		owner = &auth.Account{ID: "owner", Name: "Owner", Secret: "secret", Created: time.Now()}
		if err := auth.Create(owner); err != nil {
			t.Fatal(err)
		}
	}

	session := &Session{}
	if err := session.Rcpt(owner.ID+"@"+GetConfiguredDomain(), nil); err != nil {
		t.Fatalf("owner local recipient rejected: %v", err)
	}
	if err := session.Rcpt("other@"+GetConfiguredDomain(), nil); err == nil {
		t.Fatal("non-owner local recipient accepted")
	}
}
