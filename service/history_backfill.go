package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/model"
)

var ErrHistoryBackfillNotFound = errors.New("history backfill not found")

// BackfillSenryu associates a detected senryu with its Discord message.
// The message ID is stored only in the backfill bookkeeping table.
type BackfillSenryu struct {
	MessageID string
	Senryu    model.Senryu
}

// GetHistoryBackfill returns the saved state for a guild.
func GetHistoryBackfill(guildID string) (*model.HistoryBackfill, error) {
	var backfill model.HistoryBackfill
	if err := db.DB.Where("guild_id = ?", guildID).First(&backfill).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, ErrHistoryBackfillNotFound
		}
		return nil, fmt.Errorf("failed to get history backfill: %w", err)
	}
	return &backfill, nil
}

// GetOldestSenryuCreatedAt returns the timestamp of the oldest saved senryu.
func GetOldestSenryuCreatedAt(guildID string) (*time.Time, error) {
	var senryu model.Senryu
	err := db.DB.
		Select("created_at").
		Where("server_id = ?", guildID).
		Order("created_at ASC").
		First(&senryu).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get oldest senryu: %w", err)
	}
	cutoff := senryu.CreatedAt
	return &cutoff, nil
}

// CreateHistoryBackfill atomically creates the run and all channel cursors.
func CreateHistoryBackfill(guildID string, cutoffAt *time.Time, beforeMessageID string, channelIDs []string) error {
	tx := db.DB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin history backfill transaction: %w", tx.Error)
	}
	defer tx.Rollback()

	backfill := model.HistoryBackfill{
		GuildID:  guildID,
		CutoffAt: cutoffAt,
	}
	if err := tx.Create(&backfill).Error; err != nil {
		return fmt.Errorf("failed to create history backfill: %w", err)
	}

	for _, channelID := range channelIDs {
		channel := model.HistoryBackfillChannel{
			GuildID:         guildID,
			ChannelID:       channelID,
			BeforeMessageID: beforeMessageID,
		}
		if err := tx.Create(&channel).Error; err != nil {
			return fmt.Errorf("failed to create history backfill channel %s: %w", channelID, err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit history backfill creation: %w", err)
	}
	return nil
}

// GetNextHistoryBackfillChannel returns the next unfinished channel.
func GetNextHistoryBackfillChannel(guildID string) (*model.HistoryBackfillChannel, error) {
	var channel model.HistoryBackfillChannel
	err := db.DB.
		Where("guild_id = ? AND completed_at IS NULL", guildID).
		Order("channel_id ASC").
		First(&channel).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get next history backfill channel: %w", err)
	}
	return &channel, nil
}

// CommitHistoryBackfillPage saves all detections and advances the channel cursor atomically.
func CommitHistoryBackfillPage(
	guildID string,
	channelID string,
	detections []BackfillSenryu,
	expectedBeforeMessageID string,
	nextBeforeMessageID string,
	complete bool,
) error {
	tx := db.DB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin history backfill page transaction: %w", tx.Error)
	}
	defer tx.Rollback()

	for _, detection := range detections {
		result := tx.Exec(
			`INSERT INTO history_backfill_messages (message_id, guild_id, channel_id, created_at)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT (message_id) DO NOTHING`,
			detection.MessageID,
			guildID,
			channelID,
			time.Now().UTC(),
		)
		if result.Error != nil {
			return fmt.Errorf("failed to record backfilled message %s: %w", detection.MessageID, result.Error)
		}
		if result.RowsAffected == 0 {
			continue
		}
		if err := tx.Create(&detection.Senryu).Error; err != nil {
			return fmt.Errorf("failed to create backfilled senryu for message %s: %w", detection.MessageID, err)
		}
	}

	updates := map[string]interface{}{
		"before_message_id": nextBeforeMessageID,
		"updated_at":        time.Now().UTC(),
	}
	if complete {
		updates["completed_at"] = time.Now().UTC()
	}
	result := tx.Model(&model.HistoryBackfillChannel{}).
		Where(
			"guild_id = ? AND channel_id = ? AND before_message_id = ? AND completed_at IS NULL",
			guildID,
			channelID,
			expectedBeforeMessageID,
		).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update history backfill channel %s: %w", channelID, result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("history backfill channel %s changed concurrently, is missing, or is already complete", channelID)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit history backfill page: %w", err)
	}
	return nil
}

// CompleteHistoryBackfill marks the guild scan as complete.
func CompleteHistoryBackfill(guildID string) error {
	now := time.Now().UTC()
	result := db.DB.Model(&model.HistoryBackfill{}).
		Where("guild_id = ? AND completed_at IS NULL", guildID).
		Updates(map[string]interface{}{
			"completed_at": now,
			"updated_at":   now,
		})
	if result.Error != nil {
		return fmt.Errorf("failed to complete history backfill: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("history backfill is missing or already complete")
	}
	return nil
}
