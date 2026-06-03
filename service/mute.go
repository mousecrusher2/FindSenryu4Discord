package service

import (
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/model"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/logger"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/metrics"
)

var (
	ErrMuteFailed   = errors.New("failed to mute channel")
	ErrUnmuteFailed = errors.New("failed to unmute channel")
)

// muteCache caches muted channel IDs in memory.
// Key: channelID, Value: true (muted).
// Cache miss triggers a DB lookup and stores the result.
var muteCache sync.Map

// IsMute checks if a channel is muted
func IsMute(id string) bool {
	if cached, ok := muteCache.Load(id); ok {
		return cached.(bool)
	}

	// Cache miss — load from DB
	var muted model.MutedChannel
	isMuted := db.DB.Where(&model.MutedChannel{ChannelID: id}).First(&muted).Error == nil
	muteCache.Store(id, isMuted)
	return isMuted
}

// ToMute mutes a channel
func ToMute(channelID, guildID string) error {
	metrics.RecordDatabaseOperation("mute_channel")

	muted := model.MutedChannel{ChannelID: channelID, GuildID: guildID}
	if err := db.DB.Where("channel_id = ?", channelID).
		Assign(model.MutedChannel{GuildID: guildID}).
		FirstOrCreate(&muted).Error; err != nil {
		metrics.RecordError("database")
		logger.Error("Failed to mute channel",
			"error", err,
			"channel_id", channelID,
			"guild_id", guildID,
		)
		return errors.Wrap(err, "failed to mute channel")
	}

	muteCache.Store(channelID, true)
	logger.Info("Channel muted", "channel_id", channelID, "guild_id", guildID)
	return nil
}

// ToUnMute unmutes a channel
func ToUnMute(id string) error {
	metrics.RecordDatabaseOperation("unmute_channel")

	if err := db.DB.Where(&model.MutedChannel{ChannelID: id}).Delete(&model.MutedChannel{}).Error; err != nil {
		metrics.RecordError("database")
		logger.Error("Failed to unmute channel",
			"error", err,
			"channel_id", id,
		)
		return errors.Wrap(err, "failed to unmute channel")
	}

	muteCache.Store(id, false)
	logger.Info("Channel unmuted", "channel_id", id)
	return nil
}
