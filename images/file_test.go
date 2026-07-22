package images

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/data"
)

func TestFileHandlerServesStoredImage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	png := "\x89PNG\r\n\x1a\n0123456789"
	if err := data.SaveFile("images/generated/test.png", png); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	FileHandler(rec, httptest.NewRequest("GET", "/images/file/test.png", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != png {
		t.Error("served bytes differ from stored bytes")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "image/png") {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
}

func TestOwnerImageGenerationFailureIsNotPaymentGated(t *testing.T) {
	rec := httptest.NewRecorder()
	Handler(rec, ownerImagesRequest(t, `{"prompt":"sunset over mountains"}`))
	assertNoImagesPaymentGate(t, rec)
}

func ownerImagesRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	owner, err := auth.Owner()
	if err != nil {
		owner = &auth.Account{ID: "imagesowner", Name: "Owner", Secret: "owner-pass", Created: time.Now()}
		if err := auth.Create(owner); err != nil {
			t.Fatal(err)
		}
	}
	sess, err := auth.CreateSession(owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/images", strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	return req
}

func assertNoImagesPaymentGate(t *testing.T, recorder *httptest.ResponseRecorder) {
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

func TestFileHandlerRejectsTraversalAndMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, path := range []string{
		"/images/file/",
		"/images/file/../settings.json",
		"/images/file/nope.png",
	} {
		rec := httptest.NewRecorder()
		FileHandler(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 404 {
			t.Errorf("GET %s status = %d, want 404", path, rec.Code)
		}
	}
}
