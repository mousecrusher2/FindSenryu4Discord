package commands

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/logger"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/permissions"
	"github.com/mousecrusher2/FindSenryu4Discord/service"
)

var (
	startTime   time.Time
	allSessions []*discordgo.Session
)

// SetStartTime sets the start time for uptime calculation
func SetStartTime(t time.Time) {
	startTime = t
}

// SetAllSessions sets all shard sessions for cross-shard guild counting
func SetAllSessions(sessions []*discordgo.Session) {
	allSessions = sessions
}

// allGuilds returns guilds from all shard sessions
func allGuilds() []*discordgo.Guild {
	var guilds []*discordgo.Guild
	for _, s := range allSessions {
		if s != nil {
			guilds = append(guilds, s.State.Guilds...)
		}
	}
	return guilds
}

// AdminCommands returns the admin slash commands
func AdminCommands() []*discordgo.ApplicationCommand {
	contactMessageMaxLength := 1000
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
				{
					Name:        "contact-message",
					Description: "/contactコマンドに表示する追加メッセージを管理します",
					Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:        "set",
							Description: "追加メッセージを設定します",
							Type:        discordgo.ApplicationCommandOptionSubCommand,
							Options: []*discordgo.ApplicationCommandOption{
								{
									Name:        "message",
									Description: "表示するメッセージ",
									Type:        discordgo.ApplicationCommandOptionString,
									Required:    true,
									MaxLength:   contactMessageMaxLength,
								},
							},
						},
						{
							Name:        "clear",
							Description: "追加メッセージを削除します",
							Type:        discordgo.ApplicationCommandOptionSubCommand,
						},
						{
							Name:        "show",
							Description: "現在の追加メッセージを表示します",
							Type:        discordgo.ApplicationCommandOptionSubCommand,
						},
					},
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
	case "contact-message":
		handleContactMessageCommand(s, i, options[0].Options)
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

func handleContactMessageCommand(s *discordgo.Session, i *discordgo.InteractionCreate, options []*discordgo.ApplicationCommandInteractionDataOption) {
	if len(options) == 0 {
		respondError(s, i, "サブコマンドを指定してください")
		return
	}

	switch options[0].Name {
	case "set":
		handleContactMessageSet(s, i, options[0].Options)
	case "clear":
		handleContactMessageClear(s, i)
	case "show":
		handleContactMessageShow(s, i)
	}
}

func handleContactMessageSet(s *discordgo.Session, i *discordgo.InteractionCreate, options []*discordgo.ApplicationCommandInteractionDataOption) {
	if len(options) == 0 {
		respondError(s, i, "メッセージを指定してください")
		return
	}

	message := options[0].StringValue()
	if err := service.SetContactAdditionalMessage(message); err != nil {
		logger.Error("Failed to set contact additional message", "error", err)
		respondError(s, i, "追加メッセージの設定に失敗しました")
		return
	}

	respondEphemeral(s, i, "追加メッセージを設定しました ✅")
}

func handleContactMessageClear(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := service.ClearContactAdditionalMessage(); err != nil {
		logger.Error("Failed to clear contact additional message", "error", err)
		respondError(s, i, "追加メッセージの削除に失敗しました")
		return
	}

	respondEphemeral(s, i, "追加メッセージを削除しました ✅")
}

func handleContactMessageShow(s *discordgo.Session, i *discordgo.InteractionCreate) {
	message, err := service.GetContactAdditionalMessage()
	if err != nil {
		logger.Error("Failed to get contact additional message", "error", err)
		respondError(s, i, "追加メッセージの取得に失敗しました")
		return
	}

	if message == "" {
		respondEphemeral(s, i, "追加メッセージは設定されていません")
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "現在の追加メッセージ",
		Description: message,
		Color:       0x5865F2,
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
