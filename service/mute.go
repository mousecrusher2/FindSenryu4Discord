package service

import (
	"fmt"

	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/model"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/logger"
)

// IsMute checks if a channel is muted
func IsMute(id string) bool {
	var muted model.MutedChannel
	return db.DB.Where(&model.MutedChannel{ChannelID: id}).First(&muted).Error == nil
}

// ToMute mutes a channel
func ToMute(channelID, guildID string) error {

	muted := model.MutedChannel{ChannelID: channelID, GuildID: guildID}
	if err := db.DB.Where("channel_id = ?", channelID).
		Assign(model.MutedChannel{GuildID: guildID}).
		FirstOrCreate(&muted).Error; err != nil {
		logger.Error("Failed to mute channel",
			"error", err,
			"channel_id", channelID,
			"guild_id", guildID,
		)
		return fmt.Errorf("failed to mute channel: %w", err)
	}

	logger.Info("Channel muted", "channel_id", channelID, "guild_id", guildID)
	return nil
}

// ToUnMute unmutes a channel
func ToUnMute(id string) error {

	if err := db.DB.Where(&model.MutedChannel{ChannelID: id}).Delete(&model.MutedChannel{}).Error; err != nil {
		logger.Error("Failed to unmute channel",
			"error", err,
			"channel_id", id,
		)
		return fmt.Errorf("failed to unmute channel: %w", err)
	}

	logger.Info("Channel unmuted", "channel_id", id)
	return nil
}
