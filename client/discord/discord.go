// Package discord connects Mu to Discord as a bot. The linked Mu owner
// uses it through direct messages only.
//
// Setup:
//  1. Create a bot at https://discord.com/developers/applications
//  2. Enable Message Content Intent under Bot settings
//  3. Set DISCORD_BOT_TOKEN env var
//  4. Invite the bot to your server with the Messages scope
//
// The owner links their Discord account by sending "link <username> <password>"
// or a one-time code in a direct message.
package discord

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/settings"

	"github.com/gorilla/websocket"
)

var (
	botToken string
	botID    string
	botAppID string

	linkMu sync.RWMutex
	links  = map[string]string{} // discord user ID → mu account ID

	historyMu sync.RWMutex
	histories = map[string][]agent.QueryMessage{} // discord user ID → recent messages
)

const maxHistory = 10

func getHistory(discordID string) []agent.QueryMessage {
	historyMu.RLock()
	defer historyMu.RUnlock()
	return histories[discordID]
}

func addHistory(discordID string, role, text string) {
	historyMu.Lock()
	defer historyMu.Unlock()
	histories[discordID] = append(histories[discordID], agent.QueryMessage{Role: role, Text: text})
	if len(histories[discordID]) > maxHistory {
		histories[discordID] = histories[discordID][len(histories[discordID])-maxHistory:]
	}
}

func Load() {
	loadLinks()
	loadUsage()
	go run()
}

func loadLinks() {
	loaded := map[string]string{}
	_ = data.LoadJSON("discord_links.json", &loaded)
	linkMu.Lock()
	links = loaded
	linkMu.Unlock()
}

func Enabled() bool {
	return settings.Get("DISCORD_BOT_TOKEN") != ""
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

// LinkAccount maps a Discord user ID to a Mu account.
func LinkAccount(discordID, muAccount string) error {
	if !auth.IsOwner(muAccount) {
		return fmt.Errorf("only the Mu owner can be linked")
	}
	linkMu.Lock()
	defer linkMu.Unlock()
	links[discordID] = muAccount
	return data.SaveJSON("discord_links.json", links)
}

// GetLinkedAccount returns the Mu account for a Discord user, or "".
func GetLinkedAccount(discordID string) string {
	linkMu.RLock()
	defer linkMu.RUnlock()
	return links[discordID]
}

// DeleteLinks removes all links for a Mu account (account deletion).
func DeleteLinks(muAccount string) error {
	loadLinks()
	linkMu.Lock()
	defer linkMu.Unlock()
	for k, v := range links {
		if v == muAccount {
			delete(links, k)
		}
	}
	return data.SaveJSON("discord_links.json", links)
}

// ── Discord Gateway ──

func run() {
	for {
		token := settings.Get("DISCORD_BOT_TOKEN")
		if token == "" {
			time.Sleep(30 * time.Second)
			continue
		}
		if err := connect(token); err != nil {
			app.Log("discord", "Connection error: %v — reconnecting in 10s", err)
			time.Sleep(10 * time.Second)
		}
	}
}

func connect(token string) error {
	botToken = token

	gatewayURL, err := getGatewayURL()
	if err != nil {
		return fmt.Errorf("get gateway: %w", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(gatewayURL+"?v=10&encoding=json", nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// Read Hello
	var hello struct {
		Op int `json:"op"`
		D  struct {
			HeartbeatInterval int `json:"heartbeat_interval"`
		} `json:"d"`
	}
	if err := conn.ReadJSON(&hello); err != nil {
		return fmt.Errorf("read hello: %w", err)
	}
	if hello.Op != 10 {
		return fmt.Errorf("expected op 10, got %d", hello.Op)
	}

	// Send Identify
	identify := map[string]any{
		"op": 2,
		"d": map[string]any{
			"token":   botToken,
			"intents": 1<<9 | 1<<12 | 1<<15, // GUILD_MESSAGES | DIRECT_MESSAGES | MESSAGE_CONTENT
			"properties": map[string]string{
				"os":      "linux",
				"browser": "mu",
				"device":  "mu",
			},
		},
	}
	if err := conn.WriteJSON(identify); err != nil {
		return fmt.Errorf("send identify: %w", err)
	}

	// Start heartbeat
	ticker := time.NewTicker(time.Duration(hello.D.HeartbeatInterval) * time.Millisecond)
	defer ticker.Stop()
	var lastSeq *int

	go func() {
		for range ticker.C {
			hb := map[string]any{"op": 1, "d": lastSeq}
			conn.WriteJSON(hb)
		}
	}()

	// Read events
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var event struct {
			Op int             `json:"op"`
			T  string          `json:"t"`
			S  *int            `json:"s"`
			D  json.RawMessage `json:"d"`
		}
		json.Unmarshal(msg, &event)

		if event.S != nil {
			lastSeq = event.S
		}

		switch event.T {
		case "READY":
			var ready struct {
				User struct {
					ID       string `json:"id"`
					Username string `json:"username"`
				} `json:"user"`
				Application struct {
					ID string `json:"id"`
				} `json:"application"`
			}
			json.Unmarshal(event.D, &ready)
			botID = ready.User.ID
			botAppID = ready.Application.ID
			app.Log("discord", "Connected as %s (%s)", ready.User.Username, botID)
			go registerSlashCommands(botAppID)

		case "MESSAGE_CREATE":
			var m discordMessage
			json.Unmarshal(event.D, &m)
			go handleMessage(m)

		case "INTERACTION_CREATE":
			go handleInteraction(event.D)
		}
	}
}

type discordMessage struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Content   string `json:"content"`
	Author    struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Bot      bool   `json:"bot"`
	} `json:"author"`
	GuildID         string `json:"guild_id"`
	MentionEveryone bool   `json:"mention_everyone"`
	Mentions        []struct {
		ID string `json:"id"`
	} `json:"mentions"`
}

func handleMessage(m discordMessage) {
	// Ignore own messages
	if m.Author.Bot || m.Author.ID == botID {
		return
	}

	// Shared guild channels are never an owner communication channel.
	isDM := m.GuildID == ""
	if !isDM {
		return
	}

	app.Log("discord", "Received DM: %.100s", m.Content)

	// Strip bot mention from content
	content := m.Content
	content = strings.ReplaceAll(content, "<@"+botID+">", "")
	content = strings.ReplaceAll(content, "<@!"+botID+">", "")
	content = strings.TrimSpace(content)

	if content == "" {
		if reply := emptyMessageReply(isDM, GetLinkedAccount(m.Author.ID)); reply != "" {
			sendMessage(m.ChannelID, reply)
		}
		return
	}

	// Handle link command — one-time code or username+password
	if strings.HasPrefix(strings.ToLower(content), "link ") {
		parts := strings.Fields(content[5:])
		if len(parts) == 1 {
			// One-time code
			code := strings.TrimSpace(parts[0])
			accountID, ok := redeemCode(code)
			if !ok {
				sendMessage(m.ChannelID, "Invalid or expired code. Try `link <username> <password>` instead.")
				return
			}
			if err := LinkAccount(m.Author.ID, accountID); err != nil {
				sendMessage(m.ChannelID, "Couldn't save the account link. Try again later.")
				return
			}
			sendMessage(m.ChannelID, fmt.Sprintf("Linked to **%s**.", accountID))
			return
		} else if len(parts) >= 2 {
			username := parts[0]
			password := strings.Join(parts[1:], " ")
			if _, err := auth.Login(username, password); err != nil {
				sendMessage(m.ChannelID, "Invalid username or password.")
				return
			}
			if !auth.IsOwner(username) {
				sendMessage(m.ChannelID, "Only the Mu owner account can be linked.")
				return
			}
			if err := LinkAccount(m.Author.ID, username); err != nil {
				sendMessage(m.ChannelID, "Couldn't save the account link. Try again later.")
				return
			}
			sendMessage(m.ChannelID, fmt.Sprintf("Linked to **%s**.", username))
			return
		}
		sendMessage(m.ChannelID, "Usage: `link <code>` or DM me `link <username> <password>`")
		return
	}

	if strings.ToLower(content) == "unlink" {
		linkMu.Lock()
		delete(links, m.Author.ID)
		data.SaveJSON("discord_links.json", links)
		linkMu.Unlock()
		sendMessage(m.ChannelID, "Unlinked.")
		return
	}

	accountID := GetLinkedAccount(m.Author.ID)
	if classifyMessage(true, accountID) != accessOwner {
		sendMessage(m.ChannelID, "Link this bot to your Mu owner account before using it.")
		return
	}

	app.Log("discord", "Message from %s (%s): %s", m.Author.Username, accountID, content)
	trackQuery(accountID)

	// Show typing indicator
	showTyping(m.ChannelID)

	// Owner DMs retain the owner's private context.
	history := getHistory(m.Author.ID)
	answer, err := agent.QueryWithOpts(accountID, content, agent.QueryOpts{
		History: history,
		Public:  false,
	})
	if err != nil {
		app.Log("discord", "Agent error for %s: %v", accountID, err)
		sendMessage(m.ChannelID, "Sorry, something went wrong: "+err.Error())
		return
	}

	if strings.TrimSpace(answer) == "" {
		sendMessage(m.ChannelID, "I couldn't generate a response. Try rephrasing your question.")
		return
	}

	// Save conversation history
	addHistory(m.Author.ID, "user", content)
	addHistory(m.Author.ID, "assistant", answer)

	app.Log("discord", "Reply to %s: %.100s", m.Author.Username, answer)

	embed := formatAsEmbed(content, answer)
	sendEmbed(m.ChannelID, embed)
}

func emptyMessageReply(isDirect bool, linkedAccount string) string {
	switch classifyMessage(isDirect, linkedAccount) {
	case accessOwner:
		return "Ask me anything — I'm Micro, your agent across news, mail, markets, weather, search and more."
	case accessNeedsLink:
		return "Link this bot to your Mu owner account before using it."
	}
	return ""
}

// ── Discord HTTP API ──

func getGatewayURL() (string, error) {
	req, _ := http.NewRequest("GET", "https://discord.com/api/v10/gateway", nil)
	req.Header.Set("Authorization", "Bot "+botToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		URL string `json:"url"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.URL, nil
}

func sendMessage(channelID, content string) {
	body, _ := json.Marshal(map[string]string{"content": content})
	req, _ := http.NewRequest("POST", "https://discord.com/api/v10/channels/"+channelID+"/messages", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bot "+botToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		app.Log("discord", "Send message error: %v", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func showTyping(channelID string) {
	req, _ := http.NewRequest("POST", "https://discord.com/api/v10/channels/"+channelID+"/typing", nil)
	req.Header.Set("Authorization", "Bot "+botToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}
