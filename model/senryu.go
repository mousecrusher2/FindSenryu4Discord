package model

import "time"

// Senryu is struct of senryu.
type Senryu struct {
	ID        int       `gorm:"primaryKey;autoIncrement"`
	ServerID  string    `gorm:"column:server_id"`
	AuthorID  string    `gorm:"column:author_id"`
	Kamigo    string    `gorm:"column:kamigo"`
	Nakasichi string    `gorm:"column:nakasichi"`
	Simogo    string    `gorm:"column:simogo"`
	Spoiler   *bool     `gorm:"column:spoiler;not null"`
	CreatedAt time.Time `gorm:"column:created_at"`
}
