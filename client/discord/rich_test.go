package discord

import "testing"

func TestSlashCommandsIncludeNews(t *testing.T) {
	for _, command := range slashCommands {
		if command.Name == "news" {
			return
		}
	}
	t.Fatal("news command is not registered")
}
