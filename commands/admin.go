package commands

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/permissions"
)

var (
	startTime time.Time
	session   *discordgo.Session
)

// SetStartTime sets the start time for uptime calculation
func SetStartTime(t time.Time) {
	startTime = t
}

// SetSession sets the Discord session used for admin statistics.
func SetSession(s *discordgo.Session) {
	session = s
}

// allGuilds returns guilds from the current Discord session.
func allGuilds() []*discordgo.Guild {
	if session == nil {
		return nil
	}
	return session.State.Guilds
}

// AdminCommands returns the admin slash commands
func AdminCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "admin",
			Description: "Bot管理者向けコマンド",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "stats",
					Description: "Bot統計情報を表示します",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
	}
}

// HandleAdminCommand handles admin slash commands
func HandleAdminCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Check if user is an owner
	userID := getUserID(i)

	if !permissions.CheckOwnerPermission(userID, "admin_command") {
		respondError(s, i, "このコマンドはBot管理者のみ使用できます")
		return
	}

	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		respondError(s, i, "サブコマンドを指定してください")
		return
	}

	switch options[0].Name {
	case "stats":
		handleStatsCommand(s, i)
	}
}

func handleStatsCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	dbStats := db.GetStats()

	uptime := time.Since(startTime).Round(time.Second)

	embed := &discordgo.MessageEmbed{
		Title:     "Bot Statistics",
		Color:     0x00ff00,
		Timestamp: time.Now().Format(time.RFC3339),
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Uptime",
				Value:  uptime.String(),
				Inline: true,
			},
			{
				Name:   "Connected Guilds",
				Value:  fmt.Sprintf("%d", len(allGuilds())),
				Inline: true,
			},
			{
				Name:   "Database Driver",
				Value:  "postgres",
				Inline: true,
			},
			{
				Name:   "Total Senryus",
				Value:  fmt.Sprintf("%d", dbStats.SenryuCount),
				Inline: true,
			},
			{
				Name:   "Muted Channels",
				Value:  fmt.Sprintf("%d", dbStats.MutedChannelCount),
				Inline: true,
			},
			{
				Name:   "Database Connected",
				Value:  fmt.Sprintf("%v", dbStats.IsConnected),
				Inline: true,
			},
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

func getUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func isServerAdmin(i *discordgo.InteractionCreate) bool {
	if i.Member == nil {
		return false
	}
	return i.Member.Permissions&discordgo.PermissionAdministrator != 0
}

func canManageChannel(i *discordgo.InteractionCreate) bool {
	if i.Member == nil {
		return false
	}
	return i.Member.Permissions&(discordgo.PermissionAdministrator|discordgo.PermissionManageChannels) != 0
}

func respondError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func strPtr(s string) *string {
	return &s
}
