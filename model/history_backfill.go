package model

import "time"

// HistoryBackfill stores the fixed boundary and completion state of a guild scan.
type HistoryBackfill struct {
	GuildID     string     `gorm:"column:guild_id;primary_key"`
	CutoffAt    *time.Time `gorm:"column:cutoff_at"`
	CompletedAt *time.Time `gorm:"column:completed_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (HistoryBackfill) TableName() string {
	return "history_backfills"
}

// HistoryBackfillChannel stores the next Discord message cursor for one channel.
type HistoryBackfillChannel struct {
	GuildID         string     `gorm:"column:guild_id;primary_key"`
	ChannelID       string     `gorm:"column:channel_id;primary_key"`
	BeforeMessageID string     `gorm:"column:before_message_id"`
	CompletedAt     *time.Time `gorm:"column:completed_at"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (HistoryBackfillChannel) TableName() string {
	return "history_backfill_channels"
}

// HistoryBackfillMessage prevents one Discord message from being inserted twice
// when the same starter message is visible through multiple history endpoints.
type HistoryBackfillMessage struct {
	MessageID string    `gorm:"column:message_id;primary_key"`
	GuildID   string    `gorm:"column:guild_id;not null"`
	ChannelID string    `gorm:"column:channel_id;not null"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (HistoryBackfillMessage) TableName() string {
	return "history_backfill_messages"
}
