package service

import (
	"sync"

	"github.com/cockroachdb/errors"
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
	ErrOptOutFailed = errors.New("failed to opt out detection")
	ErrOptInFailed  = errors.New("failed to opt in detection")
	ErrAdminBanned  = errors.New("user is banned by admin")
)

// optOutCache caches detection opt-out status in memory.
// Key: "serverID:userID", Value: true (opted out).
// Cache miss triggers a DB lookup and stores the result.
var optOutCache sync.Map

// adminBanCache caches admin-ban status in memory.
// Key: "serverID:userID", Value: true (admin banned).
var adminBanCache sync.Map

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

	optOut := model.DetectionOptOut{ServerID: serverID, UserID: userID, SetBy: setBy}
	if err := db.DB.FirstOrCreate(&optOut, &model.DetectionOptOut{ServerID: serverID, UserID: userID}).Error; err != nil {
		logger.Error("Failed to opt out detection",
			"error", err,
			"server_id", serverID,
			"user_id", userID,
		)
		return errors.Wrap(err, "failed to opt out detection")
	}

	key := optOutCacheKey(serverID, userID)
	optOutCache.Store(key, true)
	adminBanCache.Store(key, setBy == SetByAdmin)
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
		return 0, errors.Wrap(result.Error, "failed to delete opt-outs by server")
	}

	// Invalidate all cache entries for this server by clearing matching keys.
	// This is acceptable because cache misses are cheap and server deletion is rare.
	prefix := serverID + ":"
	optOutCache.Range(func(key, _ any) bool {
		k := key.(string)
		if len(k) > len(serverID) && k[:len(prefix)] == prefix {
			optOutCache.Delete(key)
			adminBanCache.Delete(key)
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
		return errors.Wrap(err, "failed to opt in detection")
	}

	key := optOutCacheKey(serverID, userID)
	optOutCache.Store(key, false)
	adminBanCache.Store(key, false)
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
		return errors.Wrap(err, "failed to ban user")
	}

	optOutCache.Store(optOutCacheKey(serverID, userID), true)
	adminBanCache.Store(optOutCacheKey(serverID, userID), true)
	logger.Info("Admin banned user from detection", "server_id", serverID, "user_id", userID)
	return nil
}

// IsAdminBanned checks if a user is banned from detection by an admin.
// Results are cached to avoid repeated DB queries.
func IsAdminBanned(serverID, userID string) bool {
	key := optOutCacheKey(serverID, userID)
	if cached, ok := adminBanCache.Load(key); ok {
		return cached.(bool)
	}

	// Cache miss — load from DB
	var optOut model.DetectionOptOut
	banned := false
	if err := db.DB.Where(&model.DetectionOptOut{ServerID: serverID, UserID: userID}).First(&optOut).Error; err == nil {
		banned = optOut.SetBy == SetByAdmin
	}
	adminBanCache.Store(key, banned)
	return banned
}

// ListOptOutsByServer returns all opt-out records for a server
func ListOptOutsByServer(serverID string) ([]model.DetectionOptOut, error) {

	var optOuts []model.DetectionOptOut
	if err := db.DB.Where("server_id = ?", serverID).Find(&optOuts).Error; err != nil {
		logger.Error("Failed to list opt-outs",
			"error", err,
			"server_id", serverID,
		)
		return nil, errors.Wrap(err, "failed to list opt-outs")
	}
	return optOuts, nil
}
