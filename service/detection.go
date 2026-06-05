package service

import (
	"errors"
	"fmt"

	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/model"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/logger"
)

// SetBy constants for DetectionOptOut.SetBy column.
const (
	SetBySelf  = "self"
	SetByAdmin = "admin"
)

var (
	ErrAdminBanned = errors.New("user is banned by admin")
)

// IsDetectionOptedOut checks if a user has opted out of detection in a server
func IsDetectionOptedOut(serverID, userID string) bool {
	var optOut model.DetectionOptOut
	return db.DB.Where(&model.DetectionOptOut{ServerID: serverID, UserID: userID}).First(&optOut).Error == nil
}

// OptOutDetection opts a user out of detection in a server
func OptOutDetection(serverID, userID, setBy string) error {

	optOut := model.DetectionOptOut{ServerID: serverID, UserID: userID, SetBy: setBy}
	if err := db.DB.FirstOrCreate(&optOut, &model.DetectionOptOut{ServerID: serverID, UserID: userID}).Error; err != nil {
		logger.Error("Failed to opt out detection",
			"error", err,
			"server_id", serverID,
			"user_id", userID,
		)
		return fmt.Errorf("failed to opt out detection: %w", err)
	}

	logger.Info("User opted out of detection", "server_id", serverID, "user_id", userID, "set_by", setBy)
	return nil
}

// DeleteOptOutByServer deletes all detection opt-outs belonging to a server
func DeleteOptOutByServer(serverID string) (int64, error) {

	result := db.DB.Where("server_id = ?", serverID).Delete(&model.DetectionOptOut{})
	if result.Error != nil {
		logger.Error("Failed to delete opt-outs by server",
			"error", result.Error,
			"server_id", serverID,
		)
		return 0, fmt.Errorf("failed to delete opt-outs by server: %w", result.Error)
	}

	logger.Info("Opt-outs deleted by server",
		"server_id", serverID,
		"count", result.RowsAffected,
	)
	return result.RowsAffected, nil
}

// OptInDetection opts a user back in to detection in a server.
// If force is false (user self-service), admin-banned records are not removed.
// If force is true (admin unban), any record is removed.
func OptInDetection(serverID, userID string, force bool) error {

	if !force {
		if IsAdminBanned(serverID, userID) {
			return ErrAdminBanned
		}
	}

	if err := db.DB.Where(&model.DetectionOptOut{ServerID: serverID, UserID: userID}).Delete(&model.DetectionOptOut{}).Error; err != nil {
		logger.Error("Failed to opt in detection",
			"error", err,
			"server_id", serverID,
			"user_id", userID,
		)
		return fmt.Errorf("failed to opt in detection: %w", err)
	}

	logger.Info("User opted in to detection", "server_id", serverID, "user_id", userID, "force", force)
	return nil
}

// AdminBanDetection bans a user from detection by an admin.
// If a self opt-out already exists, it is upgraded to admin.
// Uses Assign+FirstOrCreate for atomic upsert to avoid TOCTOU races.
func AdminBanDetection(serverID, userID string) error {

	optOut := model.DetectionOptOut{ServerID: serverID, UserID: userID}
	if err := db.DB.Where(model.DetectionOptOut{ServerID: serverID, UserID: userID}).
		Assign(model.DetectionOptOut{SetBy: SetByAdmin}).
		FirstOrCreate(&optOut).Error; err != nil {
		logger.Error("Failed to admin ban user",
			"error", err,
			"server_id", serverID,
			"user_id", userID,
		)
		return fmt.Errorf("failed to ban user: %w", err)
	}

	logger.Info("Admin banned user from detection", "server_id", serverID, "user_id", userID)
	return nil
}

// IsAdminBanned checks if a user is banned from detection by an admin.
func IsAdminBanned(serverID, userID string) bool {
	var optOut model.DetectionOptOut
	if err := db.DB.Where(&model.DetectionOptOut{ServerID: serverID, UserID: userID}).First(&optOut).Error; err == nil {
		return optOut.SetBy == SetByAdmin
	}
	return false
}

// ListOptOutsByServer returns all opt-out records for a server
func ListOptOutsByServer(serverID string) ([]model.DetectionOptOut, error) {

	var optOuts []model.DetectionOptOut
	if err := db.DB.Where("server_id = ?", serverID).Find(&optOuts).Error; err != nil {
		logger.Error("Failed to list opt-outs",
			"error", err,
			"server_id", serverID,
		)
		return nil, fmt.Errorf("failed to list opt-outs: %w", err)
	}
	return optOuts, nil
}
