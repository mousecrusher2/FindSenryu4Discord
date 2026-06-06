package service

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/jinzhu/gorm"
	"github.com/mousecrusher2/FindSenryu4Discord/db"
	"github.com/mousecrusher2/FindSenryu4Discord/model"
	"github.com/mousecrusher2/FindSenryu4Discord/pkg/logger"
)

var (
	ErrSenryuNotFound = errors.New("senryu not found")
)

// CreateSenryu creates a new senryu record
func CreateSenryu(s model.Senryu) (model.Senryu, error) {

	if err := db.DB.Create(&s).Error; err != nil {
		logger.Error("Failed to create senryu",
			"error", err,
			"server_id", s.ServerID,
			"author_id", s.AuthorID,
		)
		return s, fmt.Errorf("failed to create senryu: %w", err)
	}

	logger.Debug("Senryu created",
		"id", s.ID,
		"server_id", s.ServerID,
		"author_id", s.AuthorID,
	)
	return s, nil
}

// GetLastSenryu returns the last senryu in a server
func GetLastSenryu(serverID string) (*model.Senryu, error) {

	s := model.Senryu{}
	if err := db.DB.Where(&model.Senryu{ServerID: serverID}).Last(&s).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, ErrSenryuNotFound
		}
		logger.Warn("Failed to get last senryu",
			"error", err,
			"server_id", serverID,
		)
		return nil, fmt.Errorf("failed to get last senryu: %w", err)
	}

	return &s, nil
}

// GetThreeRandomSenryus returns three random senryus for generating a new one
func GetThreeRandomSenryus(serverID string) ([]model.Senryu, error) {

	var count int64
	if err := db.DB.Model(&model.Senryu{}).Where("server_id = ? AND spoiler = ?", serverID, false).Count(&count).Error; err != nil {
		logger.Warn("Failed to count senryus",
			"error", err,
			"server_id", serverID,
		)
		return nil, fmt.Errorf("failed to count senryus: %w", err)
	}

	if count == 0 {
		return nil, nil
	}

	result := make([]model.Senryu, 0, 3)
	for i := 0; i < 3; i++ {
		var s model.Senryu
		offset := rand.Intn(int(count))
		if err := db.DB.Where("server_id = ? AND spoiler = ?", serverID, false).Offset(offset).Limit(1).First(&s).Error; err != nil {
			logger.Warn("Failed to get random senryu",
				"error", err,
				"server_id", serverID,
			)
			return nil, fmt.Errorf("failed to get random senryu: %w", err)
		}
		result = append(result, s)
	}

	return result, nil
}

// RankResult represents a ranking entry
type RankResult struct {
	Count    int
	AuthorId string
	Rank     int
}

// GetRanking returns the senryu ranking for a server
func GetRanking(serverID string) ([]RankResult, error) {

	var ranks []RankResult
	if err := db.DB.Model(&model.Senryu{}).
		Where(&model.Senryu{ServerID: serverID}).
		Group("author_id").
		Select("COUNT(TRUE) AS count, author_id").
		Order("count DESC").
		Scan(&ranks).Error; err != nil {
		logger.Warn("Failed to get ranking",
			"error", err,
			"server_id", serverID,
		)
		return nil, fmt.Errorf("failed to get ranking: %w", err)
	}

	var results []RankResult
	var before RankResult
	for i, rank := range ranks {
		if rank.Count == before.Count {
			rank.Rank = before.Rank
		} else {
			rank.Rank = i + 1
		}
		if rank.Rank > 5 {
			break
		}
		results = append(results, rank)
		before = rank
	}

	return results, nil
}

// DeleteSenryu deletes a senryu by ID within a server
func DeleteSenryu(id int, serverID string) error {

	result := db.DB.Where("id = ? AND server_id = ?", id, serverID).Delete(&model.Senryu{})
	if result.Error != nil {
		logger.Error("Failed to delete senryu",
			"error", result.Error,
			"id", id,
			"server_id", serverID,
		)
		return fmt.Errorf("failed to delete senryu: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrSenryuNotFound
	}

	logger.Info("Senryu deleted",
		"id", id,
		"server_id", serverID,
	)
	return nil
}

// GetServerStats returns statistics for a server
type ServerStats struct {
	TotalSenryus  int64
	UniqueAuthors int64
}

// GetServerStats returns statistics for a server
func GetServerStats(serverID string) (ServerStats, error) {

	var stats ServerStats

	if err := db.DB.Model(&model.Senryu{}).Where(&model.Senryu{ServerID: serverID}).Count(&stats.TotalSenryus).Error; err != nil {
		return stats, fmt.Errorf("failed to count senryus: %w", err)
	}

	var count int64
	if err := db.DB.Model(&model.Senryu{}).Where(&model.Senryu{ServerID: serverID}).Select("COUNT(DISTINCT author_id)").Count(&count).Error; err != nil {
		return stats, fmt.Errorf("failed to count unique authors: %w", err)
	}
	stats.UniqueAuthors = count

	return stats, nil
}
