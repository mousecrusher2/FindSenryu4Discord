package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"

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
	userCommands = []*discordgo.ApplicationCommand{
		{
			Name:        "rank",
			Description: "ギルド内で詠んだ回数が多い人のランキングを表示します",
		},
	}

	commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"rank": handleRankCommand,
	}
)

type exitStatus struct {
	code int
}

var (
	exitFailure = &exitStatus{code: 1}
	exitUsage   = &exitStatus{code: 2}
)

func main() {
	os.Exit(exitCode(run(os.Args[1:], os.Stdout, os.Stderr)))
}

func exitCode(status *exitStatus) int {
	if status == nil {
		return 0
	}
	return status.code
}

func run(args []string, stdout, stderr io.Writer) *exitStatus {
	command := "bot"
	if len(args) > 0 {
		command = args[0]
	}

	switch command {
	case "bot":
		return runBot()
	case "migrate":
		return runMigrate()
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		fmt.Fprintf(stderr, "<3>Unknown command: %s\n", command)
		printUsage(stderr)
		return exitUsage
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: findsenryu [bot|migrate]")
}

func runBot() (status *exitStatus) {
	// Initialize haiku dictionary
	haiku.UseDict(uni.Dict())

	// Load configuration
	conf, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "<3>Failed to load config: %v\n", err)
		return exitFailure
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
	if err := db.Init(conf.Database.DSN); err != nil {
		logger.Error("Failed to initialize database", "error", err)
		return exitFailure
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("Failed to close database", "error", err)
			status = exitFailure
		}
		logger.Info("Shutdown complete")
	}()

	// Gateway Intents
	intents := discordgo.IntentGuilds |
		discordgo.IntentGuildMessages |
		discordgo.IntentMessageContent

	dg, err := discordgo.New("Bot " + conf.Discord.Token)
	if err != nil {
		logger.Error("Failed to create Discord session", "error", err)
		return exitFailure
	}
	dg.Identify.Intents = intents
	dg.AddHandler(messageCreate)
	dg.AddHandler(interactionCreate)

	if err := dg.Open(); err != nil {
		logger.Error("Failed to open Discord connection", "error", err)
		return exitFailure
	}
	defer func() {
		if err := dg.Close(); err != nil {
			logger.Error("Failed to close Discord connection", "error", err)
			status = exitFailure
		}
	}()
	logger.Info("Discord session connected")

	// Synchronize user commands (global).
	logger.Info("Synchronizing user slash commands...")
	if _, err := dg.ApplicationCommandBulkOverwrite(dg.State.User.ID, "", userCommands); err != nil {
		logger.Error("Failed to synchronize user commands", "error", err)
	} else {
		logger.Info("Synchronized user commands", "count", len(userCommands))
	}

	logger.Info("Bot is now running. Press CTRL-C to exit.")

	// Wait for termination signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Graceful shutdown
	logger.Info("Shutting down...")

	// Slash commands are intentionally NOT removed on shutdown.
	// ApplicationCommandBulkOverwrite synchronizes them on the next startup.

	return nil
}

func runMigrate() (status *exitStatus) {
	conf, err := config.LoadMigration()
	if err != nil {
		fmt.Fprintf(os.Stderr, "<3>Failed to load config: %v\n", err)
		return exitFailure
	}

	logger.Init(logger.Config{
		Level: conf.Log.Level,
	})

	logger.Info("Starting database migration",
		"db_driver", "postgres",
	)

	if err := db.Init(conf.Database.DSN); err != nil {
		logger.Error("Failed to connect to database", "error", err)
		return exitFailure
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("Failed to close database", "error", err)
			status = exitFailure
		}
	}()

	if err := db.Migrate(); err != nil {
		logger.Error("Migration failed", "error", err)
		return exitFailure
	}

	logger.Info("Migration completed successfully")
	return nil
}

func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
		h(s, i)
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

	if !isSupportedChannelType(ch.Type) {
		return
	}

	if handleYomeYomuna(m, s) {
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
		}
	}
}

func isSupportedChannelType(channelType discordgo.ChannelType) bool {
	switch channelType {
	case discordgo.ChannelTypeGuildText,
		discordgo.ChannelTypeGuildVoice,
		discordgo.ChannelTypeGuildStageVoice,
		discordgo.ChannelTypeGuildNewsThread,
		discordgo.ChannelTypeGuildPublicThread,
		discordgo.ChannelTypeGuildPrivateThread:
		return true
	default:
		return false
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
