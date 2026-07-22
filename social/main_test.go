package social

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.RunWithTempHome(m)
}

func TestOwnerSocialSearchIsNotPaymentGated(t *testing.T) {
	rec := httptest.NewRecorder()
	handleAPISearch(rec, ownerSocialRequest(t), "hello")
	assertNoSocialPaymentGate(t, rec)
}

func ownerSocialRequest(t *testing.T) *http.Request {
	t.Helper()
	owner, err := auth.Owner()
	if err != nil {
		owner = &auth.Account{ID: "socialowner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}
		if err := auth.Create(owner); err != nil {
			t.Fatal(err)
		}
	}
	sess, err := auth.CreateSession(owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/social?query=hello", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	return req
}

func assertNoSocialPaymentGate(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code == http.StatusPaymentRequired {
		t.Fatalf("request was payment-gated: %s", recorder.Body.String())
	}
	body := strings.ToLower(recorder.Body.String())
	for _, forbidden := range []string{"insufficient credits", "top up", "/wallet"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("response contains removed payment copy %q: %s", forbidden, recorder.Body.String())
		}
	}
}
