package commands

import (
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/u16-io/FindSenryu4Discord/config"
	"github.com/u16-io/FindSenryu4Discord/pkg/logger"
	"github.com/u16-io/FindSenryu4Discord/pkg/metrics"
)

var welcomeSentGuilds sync.Map

func markGuildWelcomeSent(guildID string) {
	welcomeSentGuilds.Store(guildID, struct{}{})
}

func isGuildWelcomeSent(guildID string) bool {
	_, ok := welcomeSentGuilds.Load(guildID)
	return ok
}

func buildWelcomeEmbed() *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "FindSenryu へようこそ！",
		Description: "このBotはメッセージから川柳（五・七・五）を自動検出してお知らせします。",
		Color:       0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "川柳の検出",
				Value: "普段の会話から五・七・五のリズムを自動で見つけます。特別な操作は不要です！",
			},
			{
				Name:  "「詠め」「詠むな」",
				Value: "「詠め」と発言するとサーバー内の川柳からランダムに一句詠みます。「詠むな」で直前の自分の句を表示します。",
			},
			{
				Name:  "便利なコマンド",
				Value: "`/mute` `/unmute` — チャンネルごとの検出ON/OFF\n`/rank` — サーバー内ランキング\n`/detect off` — 自分の検出を無効化\n`/channel` — チャンネルタイプ別の設定\n`/doctor` — Bot動作の診断",
			},
		},
	}
}

func resolveWelcomeChannel(s *discordgo.Session, g *discordgo.Guild) string {
	if g.SystemChannelID != "" {
		return g.SystemChannelID
	}

	channels, err := s.GuildChannels(g.ID)
	if err != nil {
		logger.Warn("Failed to get guild channels for welcome message", "error", err, "guild_id", g.ID)
		return ""
	}

	for _, ch := range channels {
		if ch.Type != discordgo.ChannelTypeGuildText {
			continue
		}
		perms, err := s.State.UserChannelPermissions(s.State.User.ID, ch.ID)
		if err != nil {
			continue
		}
		if perms&discordgo.PermissionSendMessages != 0 {
			return ch.ID
		}
	}

	return ""
}

// SendWelcomeMessage sends a welcome embed to the guild's system channel (or first writable text channel).
func SendWelcomeMessage(s *discordgo.Session, g *discordgo.GuildCreate) {
	conf := config.GetConf()
	if !conf.Discord.IsWelcomeEnabled() {
		return
	}

	if isGuildWelcomeSent(g.ID) {
		return
	}

	channelID := resolveWelcomeChannel(s, g.Guild)
	if channelID == "" {
		logger.Warn("No writable channel found for welcome message", "guild_id", g.ID, "guild_name", g.Name)
		return
	}

	embed := buildWelcomeEmbed()
	if _, err := s.ChannelMessageSendEmbed(channelID, embed); err != nil {
		logger.Warn("Failed to send welcome message", "error", err, "guild_id", g.ID, "channel_id", channelID)
		return
	}

	markGuildWelcomeSent(g.ID)
	metrics.RecordWelcomeMessageSent()
	logger.Info("Sent welcome message", "guild_id", g.ID, "guild_name", g.Name, "channel_id", channelID)
}
