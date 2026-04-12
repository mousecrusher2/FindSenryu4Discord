package commands

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestBuildWelcomeEmbed_フィールド構成(t *testing.T) {
	embed := buildWelcomeEmbed()

	if embed.Title == "" {
		t.Error("embed title should not be empty")
	}
	if embed.Description == "" {
		t.Error("embed description should not be empty")
	}
	if embed.Color == 0 {
		t.Error("embed color should be set")
	}

	expectedFields := []string{"川柳の検出", "「詠め」「詠むな」", "便利なコマンド"}
	if len(embed.Fields) != len(expectedFields) {
		t.Fatalf("expected %d fields, got %d", len(expectedFields), len(embed.Fields))
	}
	for i, name := range expectedFields {
		if embed.Fields[i].Name != name {
			t.Errorf("field[%d].Name = %q, want %q", i, embed.Fields[i].Name, name)
		}
		if embed.Fields[i].Value == "" {
			t.Errorf("field[%d].Value should not be empty", i)
		}
	}
}

func TestBuildWelcomeEmbed_コマンド一覧を含む(t *testing.T) {
	embed := buildWelcomeEmbed()

	commandsField := embed.Fields[2]
	commands := []string{"/mute", "/unmute", "/rank", "/detect off", "/channel", "/doctor"}
	for _, cmd := range commands {
		if !containsString(commandsField.Value, cmd) {
			t.Errorf("commands field should contain %q", cmd)
		}
	}
}

func TestResolveWelcomeChannel_SystemChannelIDあり(t *testing.T) {
	guild := &discordgo.Guild{
		ID:              "guild-1",
		SystemChannelID: "system-ch-1",
	}

	s := &discordgo.Session{State: discordgo.NewState()}
	s.State.TrackChannels = true

	got := resolveWelcomeChannel(s, guild)
	if got != "system-ch-1" {
		t.Errorf("resolveWelcomeChannel() = %q, want %q", got, "system-ch-1")
	}
}

func TestResolveWelcomeChannel_SystemChannelIDが空文字(t *testing.T) {
	guild := &discordgo.Guild{
		ID:              "guild-1",
		SystemChannelID: "",
	}

	// SystemChannelID empty -> falls through to GuildChannels API call.
	// Without a real HTTP backend the API call returns an error,
	// so resolveWelcomeChannel should return "".
	s, _ := discordgo.New("Bot fake-token")
	s.State = discordgo.NewState()
	s.State.TrackChannels = true

	got := resolveWelcomeChannel(s, guild)
	if got != "" {
		t.Errorf("resolveWelcomeChannel() = %q, want empty string when API unavailable", got)
	}
}

func TestMarkAndIsGuildWelcomeSent(t *testing.T) {
	// Clean up for test isolation
	welcomeSentGuilds.Range(func(key, value any) bool {
		welcomeSentGuilds.Delete(key)
		return true
	})

	guildID := "test-guild-dedup"

	if isGuildWelcomeSent(guildID) {
		t.Error("should not be marked as sent initially")
	}

	markGuildWelcomeSent(guildID)

	if !isGuildWelcomeSent(guildID) {
		t.Error("should be marked as sent after markGuildWelcomeSent")
	}
}

func TestIsGuildWelcomeSent_異なるギルドは独立(t *testing.T) {
	welcomeSentGuilds.Range(func(key, value any) bool {
		welcomeSentGuilds.Delete(key)
		return true
	})

	markGuildWelcomeSent("guild-a")

	if !isGuildWelcomeSent("guild-a") {
		t.Error("guild-a should be marked as sent")
	}
	if isGuildWelcomeSent("guild-b") {
		t.Error("guild-b should not be marked as sent")
	}
}

func TestMarkGuildWelcomeSent_冪等性(t *testing.T) {
	welcomeSentGuilds.Range(func(key, value any) bool {
		welcomeSentGuilds.Delete(key)
		return true
	})

	guildID := "test-guild-idempotent"

	markGuildWelcomeSent(guildID)
	markGuildWelcomeSent(guildID) // should not panic

	if !isGuildWelcomeSent(guildID) {
		t.Error("should still be marked as sent after double mark")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
