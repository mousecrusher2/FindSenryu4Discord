package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/mousecrusher2/FindSenryu4Discord/commands"
	"github.com/mousecrusher2/FindSenryu4Discord/config"
	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/model"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/logger"
	"github.com/mousecrusher2/FindSenryu4Discord/service"

	"github.com/0x307e/go-haiku"
	"github.com/bwmarrin/discordgo"
	"github.com/ikawaha/kagome-dict/uni"
)

var (
	// adminPermission is used for DefaultMemberPermissions on admin-only commands.
	adminPermission int64 = discordgo.PermissionAdministrator

	// manageChannelPermission is used for DefaultMemberPermissions on channel management commands.
	manageChannelPermission int64 = discordgo.PermissionManageChannels

	userCommands = []*discordgo.ApplicationCommand{
		{
			Name:                     "mute",
			Description:              "このチャンネルでの川柳検出をミュートします",
			DefaultMemberPermissions: &manageChannelPermission,
		},
		{
			Name:                     "unmute",
			Description:              "このチャンネルでの川柳検出のミュートを解除します",
			DefaultMemberPermissions: &manageChannelPermission,
		},
		{
			Name:        "rank",
			Description: "ギルド内で詠んだ回数が多い人のランキングを表示します",
		},
		{
			Name:        "delete",
			Description: "指定ユーザーの川柳を削除します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "削除対象のユーザー",
					Required:    true,
				},
			},
		},
		{
			Name:                     "channel",
			Description:              "チャンネルタイプ別の川柳検出設定を変更します",
			DefaultMemberPermissions: &adminPermission,
		},
		{
			Name:        "doctor",
			Description: "このチャンネルでBotが正常に動作するか診断します",
		},
		{
			Name:        "detect",
			Description: "自分の川柳検出のオン/オフを切り替えます",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "on",
					Description: "川柳検出を有効にします",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "off",
					Description: "川柳検出を無効にします",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "status",
					Description: "現在の川柳検出設定を表示します",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "ban",
					Description: "指定ユーザーの川柳検出を無効化します（管理者専用）",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionUser,
							Name:        "user",
							Description: "対象ユーザー",
							Required:    true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "unban",
					Description: "指定ユーザーの川柳検出無効化を解除します（管理者専用）",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionUser,
							Name:        "user",
							Description: "対象ユーザー",
							Required:    true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "list",
					Description: "川柳検出無効化ユーザー一覧を表示します（管理者専用）",
				},
			},
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"mute":    commands.HandleMuteCommand,
		"unmute":  commands.HandleUnmuteCommand,
		"rank":    handleRankCommand,
		"channel": commands.HandleChannelCommand,
		"delete":  commands.HandleDeleteCommand,
		"doctor":  commands.HandleDoctorCommand,
		"detect":  commands.HandleDetectCommand,
	}
)

func main() {
	command := "bot"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "bot":
		runBot()
	case "migrate":
		runMigrate()
	case "help", "-h", "--help":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "<3>Unknown command: %s\n", command)
		printUsage(os.Stderr)
		os.Exit(2)
	}
}

func printUsage(w *os.File) {
	fmt.Fprintln(w, "Usage: findsenryu [bot|migrate]")
}

func runBot() {
	// Initialize haiku dictionary
	haiku.UseDict(uni.Dict())

	// Load configuration
	conf, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "<3>Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger.Init(logger.Config{
		Level: conf.Log.Level,
	})

	logger.Info("Starting FindSenryu4Discord",
		"log_level", conf.Log.Level,
		"db_driver", "postgres",
	)

	// Initialize database
	if err := db.Init(); err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}

	// Gateway Intents
	intents := discordgo.IntentGuilds |
		discordgo.IntentGuildMessages |
		discordgo.IntentMessageContent

	dg, err := discordgo.New("Bot " + conf.Discord.Token)
	if err != nil {
		logger.Error("Failed to create Discord session", "error", err)
		os.Exit(1)
	}
	dg.Identify.Intents = intents
	dg.AddHandler(messageCreate)
	dg.AddHandler(interactionCreate)
	dg.AddHandler(guildDelete)

	if err := dg.Open(); err != nil {
		logger.Error("Failed to open Discord connection", "error", err)
		os.Exit(1)
	}
	logger.Info("Discord session connected")

	// Guild-scoped commands are no longer used. Keep Discord's remote state in sync.
	for _, guild := range dg.State.Guilds {
		if _, err := dg.ApplicationCommandBulkOverwrite(dg.State.User.ID, guild.ID, []*discordgo.ApplicationCommand{}); err != nil {
			logger.Error("Failed to clear guild commands", "guild_id", guild.ID, "error", err)
		}
	}

	// Register user commands (global)
	logger.Info("Registering user slash commands...")
	for _, cmd := range userCommands {
		if _, err := dg.ApplicationCommandCreate(dg.State.User.ID, "", cmd); err != nil {
			logger.Error("Failed to register command", "command", cmd.Name, "error", err)
		} else {
			logger.Info("Registered command", "command", cmd.Name)
		}
	}

	logger.Info("Bot is now running. Press CTRL-C to exit.")

	// Wait for termination signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Graceful shutdown
	logger.Info("Shutting down...")

	// Slash commands are intentionally NOT removed on shutdown.
	// ApplicationCommandCreate (called on startup) is an upsert, so commands
	// persist across restarts without the up-to-1-hour global propagation delay.

	// Close Discord connection
	if err := dg.Close(); err != nil {
		logger.Error("Failed to close Discord connection", "error", err)
	}

	// Close database
	if err := db.Close(); err != nil {
		logger.Error("Failed to close database", "error", err)
	}

	logger.Info("Shutdown complete")
}

func runMigrate() {
	conf, err := config.LoadMigration()
	if err != nil {
		fmt.Fprintf(os.Stderr, "<3>Failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger.Init(logger.Config{
		Level: conf.Log.Level,
	})

	logger.Info("Starting database migration",
		"db_driver", "postgres",
	)

	if err := db.Init(); err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		logger.Error("Migration failed", "error", err)
		os.Exit(1)
	}

	logger.Info("Migration completed successfully")
}

func guildDelete(s *discordgo.Session, g *discordgo.GuildDelete) {
	logger.Info("Left guild", "id", g.ID)

	// Clean up guild data
	senryuCount, err := service.DeleteSenryuByServer(g.ID)
	if err != nil {
		logger.Error("Failed to clean up guild data", "error", err, "guild_id", g.ID, "type", "senryus")
	}
	optOutCount, err := service.DeleteOptOutByServer(g.ID)
	if err != nil {
		logger.Error("Failed to clean up guild data", "error", err, "guild_id", g.ID, "type", "opt-outs")
	}
	channelConfigCount, err := service.DeleteChannelConfigByGuild(g.ID)
	if err != nil {
		logger.Error("Failed to clean up guild data", "error", err, "guild_id", g.ID, "type", "channel-config")
	}

	logger.Info("Guild data cleaned up",
		"guild_id", g.ID,
		"senryus", senryuCount,
		"opt_outs", optOutCount,
		"channel_configs", channelConfigCount,
	)

}

func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
			h(s, i)
		}
	case discordgo.InteractionMessageComponent:
		handleComponentInteraction(s, i)
	}
}

func handleComponentInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID

	switch {
	case customID == commands.DeleteSelectCustomID:
		commands.HandleDeleteSelectMenu(s, i)
	case strings.HasPrefix(customID, commands.DeleteConfirmPrefix):
		commands.HandleDeleteConfirm(s, i)
	case customID == commands.DeleteCancelCustomID:
		commands.HandleDeleteCancel(s, i)
	case strings.HasPrefix(customID, commands.DeletePagePrefix):
		commands.HandleDeletePage(s, i)
	case strings.HasPrefix(customID, commands.ChannelTogglePrefix):
		commands.HandleChannelToggle(s, i)
	}
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}

	ch, err := s.State.Channel(m.ChannelID)
	if err != nil {
		ch, err = s.Channel(m.ChannelID)
		if err != nil {
			logger.Warn("Failed to get channel", "error", err, "channel_id", m.ChannelID)
			return
		}
	}

	// DM channels are not supported
	switch ch.Type {
	case discordgo.ChannelTypeDM, discordgo.ChannelTypeGroupDM:
		s.ChannelMessageSend(m.ChannelID, "個チャはダメです")
		return
	}

	// Check if this channel type is enabled for the guild
	if !service.IsChannelTypeEnabled(m.GuildID, ch.Type) {
		return
	}

	if handleYomeYomuna(m, s) {
		return
	}

	if !service.IsMute(m.ChannelID) && !isParentChannelMuted(ch) {
		if m.Author.ID != s.State.User.ID {
			if service.IsDetectionOptedOut(m.GuildID, m.Author.ID) {
				return
			}
			if containsDiscordTokens(m.Content) {
				return
			}
			content := m.Content
			spoiler := containsSpoiler(content)
			if spoiler {
				content = stripSpoilerMarkers(content)
			}
			content = stripCodeBlocks(content)
			if !isJapaneseRich(content) {
				return
			}
			h := findHaikuSafe(content, []int{5, 7, 5})
			if len(h) != 0 && !haikuSpansNewline(content, h[0]) {
				senryu := strings.Split(h[0], " ")
				created, err := service.CreateSenryu(
					model.Senryu{
						ServerID:  m.GuildID,
						AuthorID:  m.Author.ID,
						Kamigo:    senryu[0],
						Nakasichi: senryu[1],
						Simogo:    senryu[2],
						Spoiler:   &spoiler,
					},
				)
				if err != nil {
					logger.Error("Failed to create senryu", "error", err)
					return
				}
				replyText := fmt.Sprintf("川柳を検出しました！\n「%s」", h[0])
				if spoiler {
					replyText = fmt.Sprintf("川柳を検出しました！\n||「%s」||", h[0])
				}
				if _, err := s.ChannelMessageSendReply(
					m.ChannelID,
					replyText,
					m.Reference(),
				); err != nil {
					logger.Warn("Failed to send senryu reply", "error", err, "channel_id", m.ChannelID)
					// 返信に失敗した場合、保存した川柳を削除して整合性を保つ
					if delErr := service.DeleteSenryu(int(created.ID), m.GuildID); delErr != nil {
						logger.Error("Failed to rollback senryu after reply failure", "error", delErr, "senryu_id", created.ID)
					} else {
						logger.Info("Rolled back senryu after reply failure", "senryu_id", created.ID, "channel_id", m.ChannelID)
					}
					// Bot権限不足エラーの場合、該当チャンネルを自動ミュート
					if isBotPermissionError(err) {
						if muteErr := service.ToMute(m.ChannelID, m.GuildID); muteErr != nil {
							logger.Error("Failed to auto-mute channel after permission error", "error", muteErr, "channel_id", m.ChannelID)
						} else {
							logger.Warn("Auto-muted channel due to missing Bot permissions", "channel_id", m.ChannelID, "server_id", m.GuildID)
						}
					}
				}
			}
		}
	}
}

var medals = []string{"🥇", "🥈", "🥉", "🎖️", "🎖️"}

func handleRankCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {

	ranks, err := service.GetRanking(i.GuildID)
	if err != nil {
		logger.Error("Failed to get ranking", "error", err, "guild_id", i.GuildID)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "ランキングの取得に失敗しました",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	stats, statsErr := service.GetServerStats(i.GuildID)
	if statsErr != nil {
		logger.Warn("Failed to get server stats", "error", statsErr, "guild_id", i.GuildID)
	}

	guild, err := s.State.Guild(i.GuildID)
	if err != nil {
		guild, err = s.Guild(i.GuildID)
		if err != nil {
			logger.Warn("Failed to get guild for rank embed", "error", err, "guild_id", i.GuildID)
		}
	}

	embed := discordgo.MessageEmbed{
		Type:      discordgo.EmbedTypeRich,
		Title:     "サーバー内ランキング",
		Timestamp: time.Now().Format(time.RFC3339),
		Fields:    []*discordgo.MessageEmbedField{},
	}
	if statsErr == nil {
		if stats.TotalSenryus == 0 {
			embed.Description = "まだ誰も詠んでいません"
		} else {
			embed.Description = fmt.Sprintf("累計 **%d** 句 / **%d** 人の詠み手", stats.TotalSenryus, stats.UniqueAuthors)
		}
	}
	if guild != nil {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text:    guild.Name,
			IconURL: guild.IconURL(""),
		}
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: guild.IconURL(""),
		}
	}

	for _, rank := range ranks {
		member, err := s.GuildMember(i.GuildID, rank.AuthorId)
		if err != nil {
			continue
		}
		displayName := resolveDisplayName(member)
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%s 第%d位: %d回", medals[rank.Rank-1], rank.Rank, rank.Count),
			Value:  displayName,
			Inline: true,
		})
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{&embed},
		},
	})
}

func handleYomeYomuna(m *discordgo.MessageCreate, s *discordgo.Session) bool {
	switch m.Content {
	case "詠め":
		senryus, err := service.GetThreeRandomSenryus(m.GuildID)
		if err != nil {
			logger.Error("Failed to get random senryus", "error", err)
			s.MessageReactionAdd(m.ChannelID, m.ID, "❌")
			return true
		}
		if len(senryus) == 0 {
			if _, err := s.ChannelMessageSend(m.ChannelID, "まだ誰も詠んでいません。あなたが先に詠んでください。"); err != nil {
				logger.Warn("Failed to send message", "error", err, "channel_id", m.ChannelID)
			}
		} else {
			if _, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("ここで一句\n「%s」\n詠み手: %s",
				strings.Join([]string{
					senryus[0].Kamigo,
					senryus[1].Nakasichi,
					senryus[2].Simogo,
				}, " "), strings.Join(getWriters(senryus, m.GuildID, s), ", "))); err != nil {
				logger.Warn("Failed to send senryu message", "error", err, "channel_id", m.ChannelID)
			}
		}
		return true
	case "詠むな":
		senryu, err := service.GetLastSenryu(m.GuildID)
		if err != nil {
			if errors.Is(err, service.ErrSenryuNotFound) {
				s.ChannelMessageSendReply(m.ChannelID, "まだ誰も詠んでいません。", m.Reference())
			} else {
				logger.Error("Failed to get last senryu", "error", err)
				s.MessageReactionAdd(m.ChannelID, m.ID, "❌")
			}
		} else {
			var authorName string
			if senryu.AuthorID == m.Author.ID {
				authorName = "お前"
			} else {
				member, err := s.GuildMember(m.GuildID, senryu.AuthorID)
				if err != nil {
					authorName = "<@" + senryu.AuthorID + ">"
				} else {
					authorName = resolveDisplayName(member)
				}
			}
			var reply string
			if senryu.Spoiler != nil && *senryu.Spoiler {
				reply = authorName + "が||「" + senryu.Kamigo + " " + senryu.Nakasichi + " " + senryu.Simogo + "」||って詠んだのが最後やぞ"
			} else {
				reply = authorName + "が「" + senryu.Kamigo + " " + senryu.Nakasichi + " " + senryu.Simogo + "」って詠んだのが最後やぞ"
			}
			if _, err := s.ChannelMessageSendReply(
				m.ChannelID,
				reply,
				m.Reference(),
			); err != nil {
				logger.Warn("Failed to send reply", "error", err, "channel_id", m.ChannelID)
			}
		}
		return true
	}
	return false
}

// resolveDisplayName returns the best display name for a guild member,
// preferring Nick > GlobalName > Username.
func resolveDisplayName(member *discordgo.Member) string {
	if member.Nick != "" {
		return member.Nick
	}
	if member.User.GlobalName != "" {
		return member.User.GlobalName
	}
	return member.User.Username
}

// isParentChannelMuted checks if the parent channel of a thread is muted.
func isParentChannelMuted(ch *discordgo.Channel) bool {
	if ch.ParentID == "" {
		return false
	}
	return service.IsMute(ch.ParentID)
}

func sliceUnique(target []string) (unique []string) {
	m := map[string]bool{}
	for _, v := range target {
		if !m[v] {
			m[v] = true
			unique = append(unique, v)
		}
	}
	return unique
}

// containsDiscordTokens reports whether s contains Discord-specific tokens
// (mentions, channels, roles, custom emoji, URLs) that should exclude
// the message from haiku detection.
var reDiscordTokens = regexp.MustCompile(
	`<@!?\d+>` + // user mentions
		`|<#\d+>` + // channel mentions
		`|<@&\d+>` + // role mentions
		`|<a?:\w+:\d+>` + // custom emoji
		`|https?://\S+`, // URLs
)

func containsDiscordTokens(s string) bool {
	return reDiscordTokens.MatchString(s)
}

// findHaikuSafe wraps haiku.Find with recover to prevent panics from crashing the bot.
func findHaikuSafe(content string, rule []int) (result []string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Warn("Recovered from panic in haiku.Find", "error", r, "content_len", len(content))
			result = nil
		}
	}()
	return haiku.Find(content, rule)
}

var (
	reFencedCodeBlock = regexp.MustCompile("(?s)```.*?```")
	reInlineCode      = regexp.MustCompile("`[^`]+`")
)

func stripCodeBlocks(s string) string {
	s = reFencedCodeBlock.ReplaceAllString(s, "")
	s = reInlineCode.ReplaceAllString(s, "")
	return s
}

var reSpoiler = regexp.MustCompile(`\|\|.+?\|\|`)

func containsSpoiler(s string) bool {
	return reSpoiler.MatchString(s)
}

func stripSpoilerMarkers(s string) string {
	return strings.ReplaceAll(s, "||", "")
}

func haikuSpansNewline(content, haikuResult string) bool {
	if !strings.Contains(content, "\n") {
		return false
	}
	matched := strings.ReplaceAll(haikuResult, " ", "")
	return !strings.Contains(content, matched)
}

// japaneseCharRatioThreshold is the minimum ratio of Japanese characters
// (Hiragana, Katakana, Han) to total non-space characters required for a
// message to be considered "Japanese-rich" and eligible for senryu detection.
const japaneseCharRatioThreshold = 0.5

func isJapaneseRich(s string) bool {
	var total, jp int
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) ||
			r == 'ー' || // Katakana long vowel mark (U+30FC)
			r == '・' { // Katakana middle dot (U+30FB)
			jp++
		}
	}
	if total == 0 {
		return false
	}
	return float64(jp)/float64(total) >= japaneseCharRatioThreshold
}

// isBotPermissionError returns true if the error is a Discord API error
// caused by missing Bot permissions on the channel.
func isBotPermissionError(err error) bool {
	var restErr *discordgo.RESTError
	if errors.As(err, &restErr) && restErr.Message != nil {
		switch restErr.Message.Code {
		case 50001, // Missing Access
			50013,  // Missing Permissions
			160002: // Cannot reply without permission to read message history
			return true
		}
	}
	return false
}

func getWriters(senryus []model.Senryu, guildID string, session *discordgo.Session) []string {
	var writers []string
	for _, senryu := range senryus {
		member, err := session.GuildMember(guildID, senryu.AuthorID)
		if err != nil {
			continue
		}
		writers = append(writers, resolveDisplayName(member))
	}
	return sliceUnique(writers)
}
