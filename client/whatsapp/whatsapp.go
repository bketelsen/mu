// Package whatsapp connects Mu to WhatsApp via the Business Cloud API.
// Users message the bot number, and it runs the AI agent on their behalf.
//
// Setup:
//  1. Create a Meta Business account and app at developers.facebook.com
//  2. Add WhatsApp to your app, get a phone number ID and access token
//  3. Set WHATSAPP_TOKEN, WHATSAPP_PHONE_ID, WHATSAPP_VERIFY_TOKEN
//     via /admin/env
//  4. Configure the webhook URL in Meta Developer Portal:
//     https://your-domain.com/whatsapp/webhook
//
// The Mu owner links the bot with "link <username> <password>" in a direct
// message; group messages are ignored.
package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/settings"
)

const apiBase = "https://graph.facebook.com/v21.0"

var (
	linkMu sync.RWMutex
	links  = map[string]string{} // whatsapp phone number → mu account ID

	historyMu sync.RWMutex
	histories = map[string][]agent.QueryMessage{}
)

const maxHistory = 10

func Load() {
	loadLinks()
}

func loadLinks() {
	loaded := map[string]string{}
	_ = data.LoadJSON("whatsapp_links.json", &loaded)
	linkMu.Lock()
	links = loaded
	linkMu.Unlock()
}

func Enabled() bool {
	return settings.Get("WHATSAPP_TOKEN") != "" && settings.Get("WHATSAPP_PHONE_ID") != ""
}

type messageAccess = auth.MessageAccess

const (
	accessIgnore    = auth.AccessIgnore
	accessNeedsLink = auth.AccessNeedsLink
	accessOwner     = auth.AccessOwner
)

func classifyMessage(isDirect bool, linkedAccount string) messageAccess {
	return auth.ClassifyMessage(isDirect, linkedAccount)
}

// Handler handles the WhatsApp webhook at /whatsapp/webhook.
func Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		handleVerify(w, r)
	case "POST":
		handleWebhook(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleVerify handles the webhook verification challenge from Meta.
func handleVerify(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	verifyToken := settings.Get("WHATSAPP_VERIFY_TOKEN")
	if verifyToken != "" && mode == "subscribe" && token == verifyToken {
		app.Log("whatsapp", "Webhook verified")
		w.WriteHeader(200)
		w.Write([]byte(challenge))
		return
	}
	app.Log("whatsapp", "Webhook verification failed")
	w.WriteHeader(403)
}

// handleWebhook processes incoming messages from WhatsApp.
func handleWebhook(w http.ResponseWriter, r *http.Request) {
	secret := settings.Get("WHATSAPP_APP_SECRET")
	if secret == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !verifySignature(body, r.Header.Get("X-Hub-Signature-256"), secret) {
		app.Log("whatsapp", "Invalid webhook signature")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.WriteHeader(200)

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		app.Log("whatsapp", "Payload parse error: %v", err)
		return
	}

	app.Log("whatsapp", "Webhook received: %d entries", len(payload.Entry))

	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			app.Log("whatsapp", "Change: field=%s msgs=%d", change.Field, len(change.Value.Messages))
			if change.Field != "messages" {
				continue
			}
			for _, msg := range change.Value.Messages {
				app.Log("whatsapp", "Message: from=%s type=%s body=%.100s", msg.From, msg.Type, app.RedactChannelMessage(msg.Text.Body))
				if msg.Type != "text" || msg.Text.Body == "" {
					continue
				}
				isGroup := msg.GroupID != ""
				replyTo := ""
				if msg.Context != nil {
					replyTo = msg.Context.From
				}
				go handleMessage(msg.From, msg.Text.Body, isGroup, replyTo)
			}
		}
	}
}

type webhookPayload struct {
	Entry []struct {
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				Messages []struct {
					From string `json:"from"`
					Type string `json:"type"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
					Context *struct {
						From string `json:"from"`
					} `json:"context"`
					GroupID string `json:"group_id"`
				} `json:"messages"`
				Contacts []struct {
					WaID    string `json:"wa_id"`
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				} `json:"contacts"`
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

func handleMessage(from, text string, isGroup bool, replyTo string) {
	if isGroup {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	if strings.HasPrefix(strings.ToLower(text), "link ") {
		parts := strings.Fields(text[5:])
		if len(parts) >= 2 {
			username := parts[0]
			password := strings.Join(parts[1:], " ")
			if _, err := auth.Login(username, password); err != nil {
				sendMessage(from, "Invalid username or password.")
				return
			}
			if !auth.IsOwner(username) {
				sendMessage(from, "Only the Mu owner account can be linked.")
				return
			}
			if err := linkAccount(from, username); err != nil {
				sendMessage(from, "Couldn't save the account link. Try again later.")
				return
			}
			sendMessage(from, fmt.Sprintf("Linked to *%s*.", username))
			return
		}
		sendMessage(from, "Usage: link <username> <password>")
		return
	}

	if strings.ToLower(text) == "unlink" {
		linkMu.Lock()
		delete(links, from)
		data.SaveJSON("whatsapp_links.json", links)
		linkMu.Unlock()
		sendMessage(from, "Unlinked.")
		return
	}

	accountID := getLinkedAccount(from)
	if classifyMessage(true, accountID) != accessOwner {
		sendMessage(from, "Link this bot to your Mu owner account before using it.")
		return
	}

	app.Log("whatsapp", "Message from %s (%s, group=%v): %s", from, accountID, isGroup, app.RedactChannelMessage(text))

	// Owner DMs retain the owner's private context.
	history := getHistory(from)
	answer, err := agent.QueryWithOpts(accountID, text, agent.QueryOpts{
		History: history,
		Public:  false,
	})
	if err != nil {
		app.Log("whatsapp", "Agent error for %s: %v", accountID, err)
		sendMessage(from, "Sorry, something went wrong.")
		return
	}

	if strings.TrimSpace(answer) == "" {
		sendMessage(from, "I couldn't generate a response. Try rephrasing.")
		return
	}

	addHistory(from, "user", text)
	addHistory(from, "assistant", answer)

	// WhatsApp has a 4096 char limit
	if len(answer) > 4000 {
		answer = answer[:4000] + "\n…"
	}

	sendMessage(from, answer)
}

func sendMessage(to, text string) {
	token := settings.Get("WHATSAPP_TOKEN")
	phoneID := settings.Get("WHATSAPP_PHONE_ID")
	if token == "" || phoneID == "" {
		return
	}

	body, _ := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text":              map[string]string{"body": text},
	})

	url := fmt.Sprintf("%s/%s/messages", apiBase, phoneID)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		app.Log("whatsapp", "Send error: %v", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		app.Log("whatsapp", "Send failed with status %d", resp.StatusCode)
	}
}

// NotifyUser sends a message to a user's linked WhatsApp number.
func NotifyUser(muAccountID, message string) {
	if !Enabled() {
		return
	}
	linkMu.RLock()
	var phone string
	for p, mid := range links {
		if mid == muAccountID {
			phone = p
			break
		}
	}
	linkMu.RUnlock()

	if phone == "" {
		return
	}
	sendMessage(phone, message)
}

// ── Account management ──

func linkAccount(phone, muAccount string) error {
	if !auth.IsOwner(muAccount) {
		return errors.New("only the Mu owner can be linked")
	}
	linkMu.Lock()
	defer linkMu.Unlock()
	links[phone] = muAccount
	return data.SaveJSON("whatsapp_links.json", links)
}

func getLinkedAccount(phone string) string {
	linkMu.RLock()
	defer linkMu.RUnlock()
	return links[phone]
}

func DeleteLinks(muAccount string) error {
	loadLinks()
	linkMu.Lock()
	defer linkMu.Unlock()
	for k, v := range links {
		if v == muAccount {
			delete(links, k)
		}
	}
	return data.SaveJSON("whatsapp_links.json", links)
}

func getHistory(phone string) []agent.QueryMessage {
	historyMu.RLock()
	defer historyMu.RUnlock()
	return histories[phone]
}

func addHistory(phone string, role, text string) {
	historyMu.Lock()
	defer historyMu.Unlock()
	histories[phone] = append(histories[phone], agent.QueryMessage{Role: role, Text: text})
	if len(histories[phone]) > maxHistory {
		histories[phone] = histories[phone][len(histories[phone])-maxHistory:]
	}
}

func verifySignature(body []byte, signature, secret string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	sig, err := hex.DecodeString(signature[7:])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(sig, mac.Sum(nil))
}
