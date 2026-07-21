package discord

import "testing"

func TestSlashCommandsExcludeMarkets(t *testing.T) {
	for _, command := range slashCommands {
		if command.Name == "markets" {
			t.Fatalf("removed /markets command remains registered: %#v", command)
		}
	}
}
