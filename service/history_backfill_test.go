package service

import (
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/model"
)

func setupHistoryBackfillTestDB(t *testing.T) {
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

func TestGetOldestSenryuCreatedAt(t *testing.T) {
	setupHistoryBackfillTestDB(t)

	oldest := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	newest := oldest.Add(time.Hour)
	spoiler := false
	for _, createdAt := range []time.Time{newest, oldest} {
		if err := db.DB.Create(&model.Senryu{
			ServerID:  "guild",
			AuthorID:  "author",
			Kamigo:    "上の句",
			Nakasichi: "中の句です",
			Simogo:    "下の句",
			Spoiler:   &spoiler,
			CreatedAt: createdAt,
		}).Error; err != nil {
			t.Fatalf("failed to seed senryu: %v", err)
		}
	}

	got, err := GetOldestSenryuCreatedAt("guild")
	if err != nil {
		t.Fatalf("GetOldestSenryuCreatedAt() error = %v", err)
	}
	if got == nil || !got.Equal(oldest) {
		t.Fatalf("GetOldestSenryuCreatedAt() = %v, want %v", got, oldest)
	}
}

func TestCommitHistoryBackfillPageIsIdempotentAcrossChannels(t *testing.T) {
	setupHistoryBackfillTestDB(t)

	if err := CreateHistoryBackfill("guild", nil, "", []string{"channel-1", "channel-2"}); err != nil {
		t.Fatalf("CreateHistoryBackfill() error = %v", err)
	}

	spoiler := false
	detection := BackfillSenryu{
		MessageID: "message-1",
		Senryu: model.Senryu{
			ServerID:  "guild",
			AuthorID:  "author",
			Kamigo:    "古池や",
			Nakasichi: "蛙飛び込む",
			Simogo:    "水の音",
			Spoiler:   &spoiler,
			CreatedAt: time.Now().UTC(),
		},
	}
	if err := CommitHistoryBackfillPage("guild", "channel-1", []BackfillSenryu{detection}, "", "message-1", true); err != nil {
		t.Fatalf("first CommitHistoryBackfillPage() error = %v", err)
	}
	if err := CommitHistoryBackfillPage("guild", "channel-2", []BackfillSenryu{detection}, "", "message-1", true); err != nil {
		t.Fatalf("second CommitHistoryBackfillPage() error = %v", err)
	}

	var senryuCount int
	if err := db.DB.Model(&model.Senryu{}).Count(&senryuCount).Error; err != nil {
		t.Fatalf("failed to count senryus: %v", err)
	}
	if senryuCount != 1 {
		t.Fatalf("senryu count = %d, want 1", senryuCount)
	}

	var messageCount int
	if err := db.DB.Model(&model.HistoryBackfillMessage{}).Count(&messageCount).Error; err != nil {
		t.Fatalf("failed to count backfill messages: %v", err)
	}
	if messageCount != 1 {
		t.Fatalf("history backfill message count = %d, want 1", messageCount)
	}
}

func TestCommitHistoryBackfillPageRollsBackCursorAndMessage(t *testing.T) {
	setupHistoryBackfillTestDB(t)

	if err := CreateHistoryBackfill("guild", nil, "before", []string{"channel"}); err != nil {
		t.Fatalf("CreateHistoryBackfill() error = %v", err)
	}

	err := CommitHistoryBackfillPage("guild", "channel", []BackfillSenryu{
		{
			MessageID: "message",
			Senryu: model.Senryu{
				ServerID:  "guild",
				AuthorID:  "author",
				Kamigo:    "上の句",
				Nakasichi: "中の句です",
				Simogo:    "下の句",
				Spoiler:   nil,
			},
		},
	}, "before", "next", false)
	if err == nil {
		t.Fatal("CommitHistoryBackfillPage() error = nil, want transaction failure")
	}

	var messageCount int
	if err := db.DB.Model(&model.HistoryBackfillMessage{}).Count(&messageCount).Error; err != nil {
		t.Fatalf("failed to count backfill messages: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("history backfill message count = %d, want 0", messageCount)
	}

	channel, err := GetNextHistoryBackfillChannel("guild")
	if err != nil {
		t.Fatalf("GetNextHistoryBackfillChannel() error = %v", err)
	}
	if channel == nil || channel.BeforeMessageID != "before" {
		t.Fatalf("channel cursor = %v, want before", channel)
	}
}
