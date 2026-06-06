package main

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/model"
	"github.com/mousecrusher2/FindSenryu4Discord/service"
)

type fakeHistoryAPI struct {
	guilds              []*discordgo.UserGuild
	channels            []*discordgo.Channel
	activeThreads       *discordgo.ThreadsList
	publicArchived      map[string][]*discordgo.ThreadsList
	joinedPrivate       map[string][]*discordgo.ThreadsList
	publicCalls         map[string]int
	joinedPrivateCalls  map[string]int
	messagePages        map[string][][]*discordgo.Message
	messageCalls        map[string]int
	messageBefores      map[string][]string
	readableChannels    map[string]bool
	guildChannelsCalled int
}

func (f *fakeHistoryAPI) Guilds(context.Context) ([]*discordgo.UserGuild, error) {
	return f.guilds, nil
}

func (f *fakeHistoryAPI) GuildChannels(context.Context, string) ([]*discordgo.Channel, error) {
	f.guildChannelsCalled++
	return f.channels, nil
}

func (f *fakeHistoryAPI) CanReadChannel(_ context.Context, _ string, channelID string) (bool, error) {
	if f.readableChannels == nil {
		return true, nil
	}
	return f.readableChannels[channelID], nil
}

func (f *fakeHistoryAPI) ActiveThreads(context.Context, string) (*discordgo.ThreadsList, error) {
	if f.activeThreads == nil {
		return &discordgo.ThreadsList{}, nil
	}
	return f.activeThreads, nil
}

func (f *fakeHistoryAPI) PublicArchivedThreads(
	_ context.Context,
	channelID string,
	_ *time.Time,
) (*discordgo.ThreadsList, error) {
	call := f.publicCalls[channelID]
	f.publicCalls[channelID] = call + 1
	pages := f.publicArchived[channelID]
	if call >= len(pages) {
		return &discordgo.ThreadsList{}, nil
	}
	return pages[call], nil
}

func (f *fakeHistoryAPI) JoinedPrivateArchivedThreads(
	_ context.Context,
	channelID string,
	_ string,
) (*discordgo.ThreadsList, error) {
	call := f.joinedPrivateCalls[channelID]
	f.joinedPrivateCalls[channelID] = call + 1
	pages := f.joinedPrivate[channelID]
	if call >= len(pages) {
		return &discordgo.ThreadsList{}, nil
	}
	return pages[call], nil
}

func (f *fakeHistoryAPI) Messages(_ context.Context, channelID, beforeMessageID string) ([]*discordgo.Message, error) {
	call := f.messageCalls[channelID]
	f.messageCalls[channelID] = call + 1
	f.messageBefores[channelID] = append(f.messageBefores[channelID], beforeMessageID)
	pages := f.messagePages[channelID]
	if call >= len(pages) {
		return nil, errors.New("unexpected message request")
	}
	return pages[call], nil
}

func setupMainHistoryBackfillTestDB(t *testing.T) {
	t.Helper()

	testDB, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	testDB.DB().SetMaxOpenConns(1)
	db.DB = testDB
	if err := db.DB.AutoMigrate(
		&model.Senryu{},
		&model.HistoryBackfill{},
		&model.HistoryBackfillChannel{},
		&model.HistoryBackfillMessage{},
	).Error; err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}
	t.Cleanup(func() {
		db.DB.Close()
	})
}

func TestSelectSingleGuild(t *testing.T) {
	tests := []struct {
		name    string
		guilds  []*discordgo.UserGuild
		want    string
		wantErr bool
	}{
		{name: "none", wantErr: true},
		{name: "one", guilds: []*discordgo.UserGuild{{ID: "guild"}}, want: "guild"},
		{name: "multiple", guilds: []*discordgo.UserGuild{{ID: "one"}, {ID: "two"}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectSingleGuild(tt.guilds)
			if (err != nil) != tt.wantErr {
				t.Fatalf("selectSingleGuild() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("selectSingleGuild() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasHistoryPermissions(t *testing.T) {
	required := int64(discordgo.PermissionViewChannel | discordgo.PermissionReadMessageHistory)
	tests := []struct {
		name        string
		permissions int64
		want        bool
	}{
		{name: "both", permissions: required, want: true},
		{name: "administrator", permissions: int64(discordgo.PermissionAll), want: true},
		{name: "view only", permissions: int64(discordgo.PermissionViewChannel), want: false},
		{name: "history only", permissions: int64(discordgo.PermissionReadMessageHistory), want: false},
		{name: "neither", permissions: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasHistoryPermissions(tt.permissions); got != tt.want {
				t.Fatalf("hasHistoryPermissions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateHistoryChannelPermissions(t *testing.T) {
	required := int64(discordgo.PermissionViewChannel | discordgo.PermissionReadMessageHistory)
	guild := &discordgo.Guild{
		ID: "guild",
		Roles: []*discordgo.Role{
			{ID: "guild", Permissions: required},
		},
	}
	member := &discordgo.Member{User: &discordgo.User{ID: "bot"}}
	channels := []*discordgo.Channel{
		{
			ID:      "visible",
			GuildID: "guild",
			Type:    discordgo.ChannelTypeGuildText,
		},
		{
			ID:      "hidden",
			GuildID: "guild",
			Type:    discordgo.ChannelTypeGuildText,
			PermissionOverwrites: []*discordgo.PermissionOverwrite{
				{
					ID:   "guild",
					Type: discordgo.PermissionOverwriteTypeRole,
					Deny: int64(discordgo.PermissionViewChannel),
				},
			},
		},
		{
			ID:      "member-allowed",
			GuildID: "guild",
			Type:    discordgo.ChannelTypeGuildText,
			PermissionOverwrites: []*discordgo.PermissionOverwrite{
				{
					ID:   "guild",
					Type: discordgo.PermissionOverwriteTypeRole,
					Deny: required,
				},
				{
					ID:    "bot",
					Type:  discordgo.PermissionOverwriteTypeMember,
					Allow: required,
				},
			},
		},
	}

	permissions, _, err := calculateHistoryChannelPermissions("bot", guild, member, channels)
	if err != nil {
		t.Fatalf("calculateHistoryChannelPermissions() error = %v", err)
	}
	if !hasHistoryPermissions(permissions["visible"]) {
		t.Fatal("visible channel should be readable")
	}
	if hasHistoryPermissions(permissions["hidden"]) {
		t.Fatal("hidden channel should not be readable")
	}
	if !hasHistoryPermissions(permissions["member-allowed"]) {
		t.Fatal("member-allowed channel should be readable")
	}
}

func TestDiscoverHistoryChannels(t *testing.T) {
	archiveTime := time.Now().UTC()
	api := &fakeHistoryAPI{
		channels: []*discordgo.Channel{
			{ID: "text", Type: discordgo.ChannelTypeGuildText},
			{ID: "news", Type: discordgo.ChannelTypeGuildNews},
			{ID: "category", Type: discordgo.ChannelTypeGuildCategory},
		},
		activeThreads: &discordgo.ThreadsList{
			Threads: []*discordgo.Channel{
				{ID: "active", ParentID: "text", Type: discordgo.ChannelTypeGuildPublicThread},
			},
		},
		publicArchived: map[string][]*discordgo.ThreadsList{
			"text": {
				{
					Threads: []*discordgo.Channel{
						{
							ID:       "public-1",
							ParentID: "text",
							Type:     discordgo.ChannelTypeGuildPublicThread,
							ThreadMetadata: &discordgo.ThreadMetadata{
								ArchiveTimestamp: archiveTime,
							},
						},
					},
					HasMore: true,
				},
				{
					Threads: []*discordgo.Channel{
						{ID: "public-2", ParentID: "text", Type: discordgo.ChannelTypeGuildPublicThread},
					},
				},
			},
		},
		joinedPrivate: map[string][]*discordgo.ThreadsList{
			"text": {
				{
					Threads: []*discordgo.Channel{
						{ID: "private", ParentID: "text", Type: discordgo.ChannelTypeGuildPrivateThread},
					},
				},
			},
		},
		publicCalls:        make(map[string]int),
		joinedPrivateCalls: make(map[string]int),
	}

	got, err := discoverHistoryChannels(context.Background(), api, "guild")
	if err != nil {
		t.Fatalf("discoverHistoryChannels() error = %v", err)
	}
	want := []string{"active", "private", "public-1", "public-2", "text"}
	if len(got) != len(want) {
		t.Fatalf("discoverHistoryChannels() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("discoverHistoryChannels() = %v, want %v", got, want)
		}
	}
}

func TestSnowflakeAt(t *testing.T) {
	timestamp := time.UnixMilli(discordEpochMS + 123)
	if got, want := snowflakeAt(timestamp), strconv.FormatInt(123<<22, 10); got != want {
		t.Fatalf("snowflakeAt() = %q, want %q", got, want)
	}
}

func TestPublicArchivedThreadsURLPreservesSubsecondPrecision(t *testing.T) {
	before := time.Date(2026, 6, 7, 12, 34, 56, 123456789, time.FixedZone("JST", 9*60*60))
	requestURL := publicArchivedThreadsURL("channel", &before)

	parsed, err := url.Parse(requestURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	if got, want := parsed.Query().Get("before"), before.Format(time.RFC3339Nano); got != want {
		t.Fatalf("before = %q, want %q", got, want)
	}
	if got := parsed.Query().Get("limit"); got != strconv.Itoa(historyPageSize) {
		t.Fatalf("limit = %q, want %d", got, historyPageSize)
	}
}

func TestDiscordRequestRetriesRateLimit(t *testing.T) {
	calls := 0
	got, err := discordRequest(context.Background(), func(...discordgo.RequestOption) (string, error) {
		calls++
		if calls == 1 {
			return "", &discordgo.RateLimitError{
				RateLimit: &discordgo.RateLimit{
					TooManyRequests: &discordgo.TooManyRequests{RetryAfter: time.Millisecond},
					URL:             "test",
				},
			}
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("discordRequest() error = %v", err)
	}
	if got != "ok" || calls != 2 {
		t.Fatalf("discordRequest() = %q after %d calls, want ok after 2 calls", got, calls)
	}
}

func TestExecuteHistoryBackfillReturnsImmediatelyWhenComplete(t *testing.T) {
	setupMainHistoryBackfillTestDB(t)

	if err := service.CreateHistoryBackfill("guild", nil, "", nil); err != nil {
		t.Fatalf("CreateHistoryBackfill() error = %v", err)
	}
	if err := service.CompleteHistoryBackfill("guild"); err != nil {
		t.Fatalf("CompleteHistoryBackfill() error = %v", err)
	}

	api := &fakeHistoryAPI{
		guilds:             []*discordgo.UserGuild{{ID: "guild"}},
		messagePages:       make(map[string][][]*discordgo.Message),
		messageCalls:       make(map[string]int),
		messageBefores:     make(map[string][]string),
		publicCalls:        make(map[string]int),
		joinedPrivateCalls: make(map[string]int),
	}
	if err := executeHistoryBackfill(context.Background(), api); err != nil {
		t.Fatalf("executeHistoryBackfill() error = %v", err)
	}
	if api.guildChannelsCalled != 0 {
		t.Fatalf("GuildChannels() called %d times, want 0", api.guildChannelsCalled)
	}
}

func TestBackfillChannelAdvancesByPage(t *testing.T) {
	setupMainHistoryBackfillTestDB(t)

	if err := service.CreateHistoryBackfill("guild", nil, "initial", []string{"channel"}); err != nil {
		t.Fatalf("CreateHistoryBackfill() error = %v", err)
	}
	channel, err := service.GetNextHistoryBackfillChannel("guild")
	if err != nil {
		t.Fatalf("GetNextHistoryBackfillChannel() error = %v", err)
	}

	firstPage := make([]*discordgo.Message, historyPageSize)
	for i := range firstPage {
		firstPage[i] = &discordgo.Message{ID: strconv.Itoa(historyPageSize - i)}
	}
	api := &fakeHistoryAPI{
		messagePages: map[string][][]*discordgo.Message{
			"channel": {
				firstPage,
				{{ID: "0"}},
			},
		},
		messageCalls:       make(map[string]int),
		messageBefores:     make(map[string][]string),
		publicCalls:        make(map[string]int),
		joinedPrivateCalls: make(map[string]int),
	}

	if err := backfillChannel(context.Background(), api, "guild", channel); err != nil {
		t.Fatalf("backfillChannel() error = %v", err)
	}
	if got, want := api.messageBefores["channel"], []string{"initial", "1"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("message before cursors = %v, want %v", got, want)
	}
	next, err := service.GetNextHistoryBackfillChannel("guild")
	if err != nil {
		t.Fatalf("GetNextHistoryBackfillChannel() error = %v", err)
	}
	if next != nil {
		t.Fatalf("next channel = %v, want nil", next)
	}
}

func TestBackfillChannelSkipsChannelWithoutHistoryPermissions(t *testing.T) {
	setupMainHistoryBackfillTestDB(t)

	if err := service.CreateHistoryBackfill("guild", nil, "initial", []string{"hidden"}); err != nil {
		t.Fatalf("CreateHistoryBackfill() error = %v", err)
	}
	channel, err := service.GetNextHistoryBackfillChannel("guild")
	if err != nil {
		t.Fatalf("GetNextHistoryBackfillChannel() error = %v", err)
	}

	api := &fakeHistoryAPI{
		readableChannels:   map[string]bool{"hidden": false},
		messagePages:       make(map[string][][]*discordgo.Message),
		messageCalls:       make(map[string]int),
		messageBefores:     make(map[string][]string),
		publicCalls:        make(map[string]int),
		joinedPrivateCalls: make(map[string]int),
	}
	if err := backfillChannel(context.Background(), api, "guild", channel); err != nil {
		t.Fatalf("backfillChannel() error = %v", err)
	}
	if api.messageCalls["hidden"] != 0 {
		t.Fatalf("Messages() called %d times, want 0", api.messageCalls["hidden"])
	}
	next, err := service.GetNextHistoryBackfillChannel("guild")
	if err != nil {
		t.Fatalf("GetNextHistoryBackfillChannel() error = %v", err)
	}
	if next != nil {
		t.Fatalf("next channel = %v, want nil", next)
	}
}
