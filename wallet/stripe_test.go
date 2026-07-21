package wallet

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

func TestStripeWebhookDiscardsNonOwnerTargetsWithoutPersistingState(t *testing.T) {
	for _, tt := range []struct {
		name   string
		target string
		remove func(t *testing.T)
	}{
		{name: "stale target", target: "legacy"},
		{
			name:   "no owner",
			target: "owner",
			remove: func(t *testing.T) {
				t.Helper()
				owner, err := auth.Owner()
				if errors.Is(err, auth.ErrNoOwner) {
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := auth.Delete(owner); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := auth.Create(owner); err != nil {
						t.Fatal(err)
					}
				})
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ensureWebhookOwner(t)
			if tt.remove != nil {
				tt.remove(t)
			}

			mutex.Lock()
			oldWallets, oldTransactions, oldProcessed := wallets, transactions, processedSessions
			wallets, transactions, processedSessions = map[string]*Wallet{}, map[string][]*Transaction{}, map[string]bool{}
			mutex.Unlock()
			t.Cleanup(func() {
				mutex.Lock()
				wallets, transactions, processedSessions = oldWallets, oldTransactions, oldProcessed
				mutex.Unlock()
			})

			eventID := "checkout-" + strings.ReplaceAll(tt.name, " ", "-")
			req := signedCheckoutWebhook(t, eventID, tt.target)
			rr := httptest.NewRecorder()
			HandleStripeWebhook(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("webhook status = %d, want %d", rr.Code, http.StatusOK)
			}

			mutex.Lock()
			walletCount, transactionCount := len(wallets), len(transactions)
			_, recorded := processedSessions[eventID]
			mutex.Unlock()
			if walletCount != 0 || transactionCount != 0 {
				t.Fatalf("webhook persisted wallet state for discarded target %q: wallets=%d transactions=%d", tt.target, walletCount, transactionCount)
			}
			if recorded {
				t.Fatalf("webhook persisted dedup marker for discarded target %q", tt.target)
			}
		})
	}
}

func ensureWebhookOwner(t *testing.T) {
	t.Helper()
	if _, err := auth.Owner(); errors.Is(err, auth.ErrNoOwner) {
		owner := &auth.Account{ID: "owner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}
		if err := auth.Create(owner); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := auth.Delete(owner); err != nil {
				t.Fatal(err)
			}
		})
	} else if err != nil {
		t.Fatal(err)
	}
}

func signedCheckoutWebhook(t *testing.T, id, target string) *http.Request {
	t.Helper()
	secret := "stripe-test-secret"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)
	body := fmt.Sprintf(`{"type":"checkout.session.completed","data":{"object":{"id":%q,"payment_status":"paid","metadata":{"user_id":%q,"credits":"100"},"amount_total":100}}}`, id, target)
	timestamp := fmt.Sprint(time.Now().Unix())
	req := httptest.NewRequest(http.MethodPost, "/wallet/stripe/webhook", strings.NewReader(body))
	req.Header.Set("Stripe-Signature", "t="+timestamp+",v1="+computeHMAC(timestamp+"."+body, secret))
	return req
}
