package service

import (
	"fmt"
	"sync/atomic"

	"github.com/jinzhu/gorm"
	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/model"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/logger"
)

const metadataKeyContactAdditionalMessage = "contact_additional_message"

// contactAdditionalMessageCache caches the additional message in memory.
// Stored value is *string: nil (zero Value) = not loaded yet,
// empty *string = DB has no row, non-empty *string = cached value.
var contactAdditionalMessageCache atomic.Value

// GetContactAdditionalMessage returns the additional message for the /contact command.
// Returns empty string if not set.
func GetContactAdditionalMessage() (string, error) {
	if cached := contactAdditionalMessageCache.Load(); cached != nil {
		return *cached.(*string), nil
	}

	var meta model.Metadata
	err := db.DB.Where("key = ?", metadataKeyContactAdditionalMessage).First(&meta).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			empty := ""
			contactAdditionalMessageCache.Store(&empty)
			return "", nil
		}
		logger.Error("Failed to get contact additional message", "error", err)
		return "", fmt.Errorf("failed to get contact additional message: %w", err)
	}

	contactAdditionalMessageCache.Store(&meta.Value)
	return meta.Value, nil
}

// SetContactAdditionalMessage sets the additional message for the /contact command.
func SetContactAdditionalMessage(message string) error {

	meta := model.Metadata{Key: metadataKeyContactAdditionalMessage, Value: message}
	if err := db.DB.Where("key = ?", metadataKeyContactAdditionalMessage).
		Assign(model.Metadata{Value: message}).
		FirstOrCreate(&meta).Error; err != nil {
		logger.Error("Failed to set contact additional message", "error", err)
		return fmt.Errorf("failed to set contact additional message: %w", err)
	}

	msg := message
	contactAdditionalMessageCache.Store(&msg)
	logger.Info("Contact additional message updated", "message_length", len(message))
	return nil
}

// ClearContactAdditionalMessage removes the additional message for the /contact command.
func ClearContactAdditionalMessage() error {

	if err := db.DB.Where("key = ?", metadataKeyContactAdditionalMessage).
		Delete(&model.Metadata{}).Error; err != nil {
		logger.Error("Failed to clear contact additional message", "error", err)
		return fmt.Errorf("failed to clear contact additional message: %w", err)
	}

	empty := ""
	contactAdditionalMessageCache.Store(&empty)
	logger.Info("Contact additional message cleared")
	return nil
}
