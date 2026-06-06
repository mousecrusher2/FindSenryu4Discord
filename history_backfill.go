package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/0x307e/go-haiku"
	"github.com/bwmarrin/discordgo"
	"github.com/ikawaha/kagome-dict/uni"
	"github.com/mousecrusher2/FindSenryu4Discord/config"
	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/model"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/logger"
	"github.com/mousecrusher2/FindSenryu4Discord/service"
)

const (
	historyPageSize = 100
	discordEpochMS  = int64(1420070400000)
)

type historyAPI interface {
	Guilds(context.Context) ([]*discordgo.UserGuild, error)
	GuildChannels(context.Context, string) ([]*discordgo.Channel, error)
	ActiveThreads(context.Context, string) (*discordgo.ThreadsList, error)
	PublicArchivedThreads(context.Context, string, *time.Time) (*discordgo.ThreadsList, error)
	JoinedPrivateArchivedThreads(context.Context, string, string) (*discordgo.ThreadsList, error)
	Messages(context.Context, string, string) ([]*discordgo.Message, error)
}

type discordHistoryAPI struct {
	session *discordgo.Session
}

func newDiscordHistoryAPI(token string) (*discordHistoryAPI, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}
	session.ShouldRetryOnRateLimit = false
	return &discordHistoryAPI{session: session}, nil
}

func (a *discordHistoryAPI) Guilds(ctx context.Context) ([]*discordgo.UserGuild, error) {
	return discordRequest(ctx, func(options ...discordgo.RequestOption) ([]*discordgo.UserGuild, error) {
		return a.session.UserGuilds(2, "", "", false, options...)
	})
}

func (a *discordHistoryAPI) GuildChannels(ctx context.Context, guildID string) ([]*discordgo.Channel, error) {
	return discordRequest(ctx, func(options ...discordgo.RequestOption) ([]*discordgo.Channel, error) {
		return a.session.GuildChannels(guildID, options...)
	})
}

func (a *discordHistoryAPI) ActiveThreads(ctx context.Context, guildID string) (*discordgo.ThreadsList, error) {
	return discordRequest(ctx, func(options ...discordgo.RequestOption) (*discordgo.ThreadsList, error) {
		return a.session.GuildThreadsActive(guildID, options...)
	})
}

func (a *discordHistoryAPI) PublicArchivedThreads(
	ctx context.Context,
	channelID string,
	before *time.Time,
) (*discordgo.ThreadsList, error) {
	requestURL := publicArchivedThreadsURL(channelID, before)
	body, err := discordRequest(ctx, func(options ...discordgo.RequestOption) ([]byte, error) {
		return a.session.Request("GET", requestURL, nil, options...)
	})
	if err != nil {
		return nil, err
	}

	var threads discordgo.ThreadsList
	if err := json.Unmarshal(body, &threads); err != nil {
		return nil, fmt.Errorf("failed to decode public archived threads: %w", err)
	}
	return &threads, nil
}

func publicArchivedThreadsURL(channelID string, before *time.Time) string {
	endpoint := discordgo.EndpointChannelPublicArchivedThreads(channelID)
	query := url.Values{}
	query.Set("limit", strconv.Itoa(historyPageSize))
	if before != nil {
		query.Set("before", before.Format(time.RFC3339Nano))
	}
	return endpoint + "?" + query.Encode()
}

func (a *discordHistoryAPI) JoinedPrivateArchivedThreads(
	ctx context.Context,
	channelID string,
	beforeThreadID string,
) (*discordgo.ThreadsList, error) {
	endpoint := discordgo.EndpointChannelJoinedPrivateArchivedThreads(channelID)
	query := url.Values{}
	query.Set("limit", strconv.Itoa(historyPageSize))
	if beforeThreadID != "" {
		query.Set("before", beforeThreadID)
	}
	requestURL := endpoint + "?" + query.Encode()

	body, err := discordRequest(ctx, func(options ...discordgo.RequestOption) ([]byte, error) {
		return a.session.Request("GET", requestURL, nil, options...)
	})
	if err != nil {
		return nil, err
	}

	var threads discordgo.ThreadsList
	if err := json.Unmarshal(body, &threads); err != nil {
		return nil, fmt.Errorf("failed to decode joined private archived threads: %w", err)
	}
	return &threads, nil
}

func (a *discordHistoryAPI) Messages(
	ctx context.Context,
	channelID string,
	beforeMessageID string,
) ([]*discordgo.Message, error) {
	return discordRequest(ctx, func(options ...discordgo.RequestOption) ([]*discordgo.Message, error) {
		return a.session.ChannelMessages(channelID, historyPageSize, beforeMessageID, "", "", options...)
	})
}

type discordResult[T any] struct {
	value T
	err   error
}

func discordRequest[T any](
	ctx context.Context,
	request func(...discordgo.RequestOption) (T, error),
) (T, error) {
	var zero T
	for {
		resultChannel := make(chan discordResult[T], 1)
		go func() {
			value, err := request(
				discordgo.WithContext(ctx),
				discordgo.WithRetryOnRatelimit(false),
			)
			resultChannel <- discordResult[T]{value: value, err: err}
		}()

		var result discordResult[T]
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case result = <-resultChannel:
		}

		var rateLimitError *discordgo.RateLimitError
		if !errors.As(result.err, &rateLimitError) {
			return result.value, result.err
		}

		logger.Warn("Discord rate limit reached", "retry_after", rateLimitError.RetryAfter)
		timer := time.NewTimer(rateLimitError.RetryAfter)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return zero, ctx.Err()
		case <-timer.C:
		}
	}
}

func runBackfill() (status *exitStatus) {
	haiku.UseDict(uni.Dict())

	conf, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "<3>Failed to load config: %v\n", err)
		return exitFailure
	}

	logger.Init(logger.Config{Level: conf.Log.Level})
	logger.Info("Starting Discord history backfill")

	if err := db.Init(conf.Database.DSN); err != nil {
		logger.Error("Failed to initialize database", "error", err)
		return exitFailure
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("Failed to close database", "error", err)
			status = exitFailure
		}
	}()

	api, err := newDiscordHistoryAPI(conf.Discord.Token)
	if err != nil {
		logger.Error("Failed to initialize Discord REST client", "error", err)
		return exitFailure
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer stop()

	if err := executeHistoryBackfill(ctx, api); err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Warn("History backfill interrupted; run the command again to resume")
		} else {
			logger.Error("History backfill failed", "error", err)
		}
		return exitFailure
	}

	logger.Info("History backfill completed")
	return nil
}

func executeHistoryBackfill(ctx context.Context, api historyAPI) error {
	guilds, err := api.Guilds(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Discord guilds: %w", err)
	}
	guildID, err := selectSingleGuild(guilds)
	if err != nil {
		return err
	}

	backfill, err := service.GetHistoryBackfill(guildID)
	switch {
	case err == nil && backfill.CompletedAt != nil:
		logger.Info("History backfill is already complete", "guild_id", guildID)
		return nil
	case err == nil:
		logger.Info("Resuming history backfill", "guild_id", guildID)
	case errors.Is(err, service.ErrHistoryBackfillNotFound):
		if err := initializeHistoryBackfill(ctx, api, guildID); err != nil {
			return err
		}
	default:
		return err
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		channel, err := service.GetNextHistoryBackfillChannel(guildID)
		if err != nil {
			return err
		}
		if channel == nil {
			break
		}
		if err := backfillChannel(ctx, api, guildID, channel); err != nil {
			return err
		}
	}

	if err := service.CompleteHistoryBackfill(guildID); err != nil {
		return err
	}
	return nil
}

func selectSingleGuild(guilds []*discordgo.UserGuild) (string, error) {
	switch len(guilds) {
	case 0:
		return "", errors.New("the Discord bot is not a member of any guild")
	case 1:
		return guilds[0].ID, nil
	default:
		return "", fmt.Errorf("the Discord bot is a member of %d or more guilds; exactly one is required", len(guilds))
	}
}

func initializeHistoryBackfill(ctx context.Context, api historyAPI, guildID string) error {
	cutoffAt, err := service.GetOldestSenryuCreatedAt(guildID)
	if err != nil {
		return err
	}

	channelIDs, err := discoverHistoryChannels(ctx, api, guildID)
	if err != nil {
		return fmt.Errorf("failed to discover history channels: %w", err)
	}

	beforeMessageID := ""
	if cutoffAt != nil {
		beforeMessageID = snowflakeAt(*cutoffAt)
	}
	if err := service.CreateHistoryBackfill(guildID, cutoffAt, beforeMessageID, channelIDs); err != nil {
		return err
	}

	logger.Info("Initialized history backfill",
		"guild_id", guildID,
		"channels", len(channelIDs),
		"cutoff_at", cutoffAt,
	)
	return nil
}

func discoverHistoryChannels(ctx context.Context, api historyAPI, guildID string) ([]string, error) {
	channels, err := api.GuildChannels(ctx, guildID)
	if err != nil {
		return nil, fmt.Errorf("failed to get guild channels: %w", err)
	}

	targets := make(map[string]struct{})
	for _, channel := range channels {
		if isSupportedChannelType(channel.Type) {
			targets[channel.ID] = struct{}{}
		}
	}

	activeThreads, err := api.ActiveThreads(ctx, guildID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active threads: %w", err)
	}
	addThreads(targets, activeThreads.Threads)

	for _, channel := range channels {
		if isPublicThreadParent(channel.Type) {
			if err := discoverPublicArchivedThreads(ctx, api, channel.ID, targets); err != nil {
				return nil, err
			}
		}
		if channel.Type == discordgo.ChannelTypeGuildText {
			if err := discoverJoinedPrivateArchivedThreads(ctx, api, channel.ID, targets); err != nil {
				return nil, err
			}
		}
	}

	channelIDs := make([]string, 0, len(targets))
	for channelID := range targets {
		channelIDs = append(channelIDs, channelID)
	}
	sort.Strings(channelIDs)
	return channelIDs, nil
}

func discoverPublicArchivedThreads(
	ctx context.Context,
	api historyAPI,
	channelID string,
	targets map[string]struct{},
) error {
	var before *time.Time
	for {
		threads, err := api.PublicArchivedThreads(ctx, channelID, before)
		if err != nil {
			return fmt.Errorf("failed to get public archived threads for channel %s: %w", channelID, err)
		}
		addThreads(targets, threads.Threads)
		if !threads.HasMore {
			return nil
		}
		if len(threads.Threads) == 0 {
			return fmt.Errorf("public archived thread pagination for channel %s returned has_more without threads", channelID)
		}
		last := threads.Threads[len(threads.Threads)-1]
		if last.ThreadMetadata == nil || last.ThreadMetadata.ArchiveTimestamp.IsZero() {
			return fmt.Errorf("public archived thread %s has no archive timestamp", last.ID)
		}
		nextBefore := last.ThreadMetadata.ArchiveTimestamp
		before = &nextBefore
	}
}

func discoverJoinedPrivateArchivedThreads(
	ctx context.Context,
	api historyAPI,
	channelID string,
	targets map[string]struct{},
) error {
	beforeThreadID := ""
	for {
		threads, err := api.JoinedPrivateArchivedThreads(ctx, channelID, beforeThreadID)
		if err != nil {
			return fmt.Errorf("failed to get joined private archived threads for channel %s: %w", channelID, err)
		}
		addThreads(targets, threads.Threads)
		if !threads.HasMore {
			return nil
		}
		if len(threads.Threads) == 0 {
			return fmt.Errorf("joined private archived thread pagination for channel %s returned has_more without threads", channelID)
		}
		nextBefore := threads.Threads[len(threads.Threads)-1].ID
		if nextBefore == "" || nextBefore == beforeThreadID {
			return fmt.Errorf("joined private archived thread pagination for channel %s did not advance", channelID)
		}
		beforeThreadID = nextBefore
	}
}

func addThreads(targets map[string]struct{}, threads []*discordgo.Channel) {
	for _, thread := range threads {
		if thread != nil && isSupportedChannelType(thread.Type) {
			targets[thread.ID] = struct{}{}
		}
	}
}

func isPublicThreadParent(channelType discordgo.ChannelType) bool {
	switch channelType {
	case discordgo.ChannelTypeGuildText,
		discordgo.ChannelTypeGuildNews,
		discordgo.ChannelTypeGuildForum,
		discordgo.ChannelTypeGuildMedia:
		return true
	default:
		return false
	}
}

func backfillChannel(
	ctx context.Context,
	api historyAPI,
	guildID string,
	channel *model.HistoryBackfillChannel,
) error {
	beforeMessageID := channel.BeforeMessageID
	logger.Info("Backfilling channel", "channel_id", channel.ChannelID, "before_message_id", beforeMessageID)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		messages, err := api.Messages(ctx, channel.ChannelID, beforeMessageID)
		if err != nil {
			return fmt.Errorf("failed to get messages for channel %s: %w", channel.ChannelID, err)
		}

		detections := make([]service.BackfillSenryu, 0)
		for _, message := range messages {
			if err := ctx.Err(); err != nil {
				return err
			}
			detection := detectHistoricalSenryu(guildID, message)
			if detection != nil {
				detections = append(detections, *detection)
			}
		}

		complete := len(messages) < historyPageSize
		nextBeforeMessageID := beforeMessageID
		if len(messages) > 0 {
			nextBeforeMessageID = messages[len(messages)-1].ID
			if nextBeforeMessageID == "" || nextBeforeMessageID == beforeMessageID {
				return fmt.Errorf("message pagination for channel %s did not advance", channel.ChannelID)
			}
		}

		if err := service.CommitHistoryBackfillPage(
			guildID,
			channel.ChannelID,
			detections,
			beforeMessageID,
			nextBeforeMessageID,
			complete,
		); err != nil {
			return err
		}

		logger.Info("Committed history page",
			"channel_id", channel.ChannelID,
			"messages", len(messages),
			"senryus", len(detections),
			"complete", complete,
		)
		if complete {
			return nil
		}
		beforeMessageID = nextBeforeMessageID
	}
}

func detectHistoricalSenryu(guildID string, message *discordgo.Message) *service.BackfillSenryu {
	if message == nil || message.Author == nil || message.Author.Bot {
		return nil
	}
	if message.Content == "詠め" || message.Content == "詠むな" {
		return nil
	}

	detection := detectSenryu(message.Content)
	if detection == nil {
		return nil
	}
	spoiler := detection.Spoiler
	return &service.BackfillSenryu{
		MessageID: message.ID,
		Senryu: model.Senryu{
			ServerID:  guildID,
			AuthorID:  message.Author.ID,
			Kamigo:    detection.Kamigo,
			Nakasichi: detection.Nakasichi,
			Simogo:    detection.Simogo,
			Spoiler:   &spoiler,
			CreatedAt: message.Timestamp,
		},
	}
}

func snowflakeAt(timestamp time.Time) string {
	milliseconds := timestamp.UTC().UnixMilli()
	if milliseconds <= discordEpochMS {
		return "0"
	}
	return strconv.FormatInt((milliseconds-discordEpochMS)<<22, 10)
}
