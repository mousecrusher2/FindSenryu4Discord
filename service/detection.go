package service

import (
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/u16-io/FindSenryu4Discord/db"
	"github.com/u16-io/FindSenryu4Discord/model"
	"github.com/u16-io/FindSenryu4Discord/pkg/logger"
	"github.com/u16-io/FindSenryu4Discord/pkg/metrics"
)

var (
	ErrOptOutFailed = errors.New("failed to opt out detection")
	ErrOptInFailed  = errors.New("failed to opt in detection")
	ErrAdminBanned  = errors.New("user is banned by admin")
	ErrBanFailed    = errors.New("failed to ban user")
	ErrListOptOuts  = errors.New("failed to list opt-outs")
)

// optOutCache caches detection opt-out status in memory.
// Key: "serverID:userID", Value: true (opted out).
// Cache miss triggers a DB lookup and stores the result.
var optOutCache sync.Map

func optOutCacheKey(serverID, userID string) string {
	return serverID + ":" + userID
}

// IsDetectionOptedOut checks if a user has opted out of detection in a server
func IsDetectionOptedOut(serverID, userID string) bool {
	key := optOutCacheKey(serverID, userID)
	if cached, ok := optOutCache.Load(key); ok {
		return cached.(bool)
	}

	// Cache miss — load from DB
	var optOut model.DetectionOptOut
	isOptedOut := db.DB.Where(&model.DetectionOptOut{ServerID: serverID, UserID: userID}).First(&optOut).Error == nil
	optOutCache.Store(key, isOptedOut)
	return isOptedOut
}

// OptOutDetection opts a user out of detection in a server
func OptOutDetection(serverID, userID, setBy string) error {
	metrics.RecordDatabaseOperation("opt_out_detection")

	optOut := model.DetectionOptOut{ServerID: serverID, UserID: userID, SetBy: setBy}
	if err := db.DB.FirstOrCreate(&optOut, &model.DetectionOptOut{ServerID: serverID, UserID: userID}).Error; err != nil {
		metrics.RecordError("database")
		logger.Error("Failed to opt out detection",
			"error", err,
			"server_id", serverID,
			"user_id", userID,
		)
		return errors.Wrap(err, "failed to opt out detection")
	}

	optOutCache.Store(optOutCacheKey(serverID, userID), true)
	logger.Info("User opted out of detection", "server_id", serverID, "user_id", userID, "set_by", setBy)
	return nil
}

// DeleteOptOutByServer deletes all detection opt-outs belonging to a server
func DeleteOptOutByServer(serverID string) (int64, error) {
	metrics.RecordDatabaseOperation("delete_opt_out_by_server")

	result := db.DB.Where("server_id = ?", serverID).Delete(&model.DetectionOptOut{})
	if result.Error != nil {
		metrics.RecordError("database")
		logger.Error("Failed to delete opt-outs by server",
			"error", result.Error,
			"server_id", serverID,
		)
		return 0, errors.Wrap(result.Error, "failed to delete opt-outs by server")
	}

	// Invalidate all cache entries for this server by clearing entire cache.
	// This is acceptable because cache misses are cheap and server deletion is rare.
	optOutCache.Range(func(key, _ any) bool {
		k := key.(string)
		if len(k) > len(serverID) && k[:len(serverID)+1] == serverID+":" {
			optOutCache.Delete(key)
		}
		return true
	})

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
	metrics.RecordDatabaseOperation("opt_in_detection")

	if !force {
		if IsAdminBanned(serverID, userID) {
			return ErrAdminBanned
		}
	}

	if err := db.DB.Where(&model.DetectionOptOut{ServerID: serverID, UserID: userID}).Delete(&model.DetectionOptOut{}).Error; err != nil {
		metrics.RecordError("database")
		logger.Error("Failed to opt in detection",
			"error", err,
			"server_id", serverID,
			"user_id", userID,
		)
		return errors.Wrap(err, "failed to opt in detection")
	}

	optOutCache.Store(optOutCacheKey(serverID, userID), false)
	logger.Info("User opted in to detection", "server_id", serverID, "user_id", userID, "force", force)
	return nil
}

// AdminBanDetection bans a user from detection by an admin.
// If a self opt-out already exists, it is upgraded to admin.
func AdminBanDetection(serverID, userID string) error {
	metrics.RecordDatabaseOperation("admin_ban_detection")

	var existing model.DetectionOptOut
	if err := db.DB.Where(&model.DetectionOptOut{ServerID: serverID, UserID: userID}).First(&existing).Error; err == nil {
		// Record exists — update set_by to admin
		if existing.SetBy != "admin" {
			if err := db.DB.Model(&existing).Update("set_by", "admin").Error; err != nil {
				metrics.RecordError("database")
				logger.Error("Failed to update opt-out to admin ban",
					"error", err,
					"server_id", serverID,
					"user_id", userID,
				)
				return errors.Wrap(err, "failed to ban user")
			}
		}
	} else {
		// Create new admin ban record
		optOut := model.DetectionOptOut{ServerID: serverID, UserID: userID, SetBy: "admin"}
		if err := db.DB.Create(&optOut).Error; err != nil {
			metrics.RecordError("database")
			logger.Error("Failed to create admin ban",
				"error", err,
				"server_id", serverID,
				"user_id", userID,
			)
			return errors.Wrap(err, "failed to ban user")
		}
	}

	optOutCache.Store(optOutCacheKey(serverID, userID), true)
	logger.Info("Admin banned user from detection", "server_id", serverID, "user_id", userID)
	return nil
}

// IsAdminBanned checks if a user is banned from detection by an admin
func IsAdminBanned(serverID, userID string) bool {
	var optOut model.DetectionOptOut
	if err := db.DB.Where(&model.DetectionOptOut{ServerID: serverID, UserID: userID}).First(&optOut).Error; err != nil {
		return false
	}
	return optOut.SetBy == "admin"
}

// ListOptOutsByServer returns all opt-out records for a server
func ListOptOutsByServer(serverID string) ([]model.DetectionOptOut, error) {
	metrics.RecordDatabaseOperation("list_opt_outs_by_server")

	var optOuts []model.DetectionOptOut
	if err := db.DB.Where("server_id = ?", serverID).Find(&optOuts).Error; err != nil {
		metrics.RecordError("database")
		logger.Error("Failed to list opt-outs",
			"error", err,
			"server_id", serverID,
		)
		return nil, errors.Wrap(err, "failed to list opt-outs")
	}
	return optOuts, nil
}
