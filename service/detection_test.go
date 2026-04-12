package service

import (
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/u16-io/FindSenryu4Discord/db"
	"github.com/u16-io/FindSenryu4Discord/model"
)

func setupDetectionTestDB(t *testing.T) {
	t.Helper()
	var err error
	db.DB, err = gorm.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	db.DB.AutoMigrate(&model.DetectionOptOut{})
	// Clear in-memory caches for test isolation
	optOutCache.Range(func(key, _ interface{}) bool {
		optOutCache.Delete(key)
		return true
	})
	adminBanCache.Range(func(key, _ interface{}) bool {
		adminBanCache.Delete(key)
		return true
	})
	t.Cleanup(func() {
		db.DB.Close()
	})
}

func TestAdminBanDetection_レコードが作成されること(t *testing.T) {
	setupDetectionTestDB(t)

	if err := AdminBanDetection("server1", "user1"); err != nil {
		t.Fatalf("AdminBanDetection failed: %v", err)
	}

	var optOut model.DetectionOptOut
	if err := db.DB.Where("server_id = ? AND user_id = ?", "server1", "user1").First(&optOut).Error; err != nil {
		t.Fatalf("failed to find opt-out record: %v", err)
	}
	if optOut.SetBy != SetByAdmin {
		t.Errorf("expected set_by='admin', got %q", optOut.SetBy)
	}
}

func TestAdminBan後にDetectOnで解除できないこと(t *testing.T) {
	setupDetectionTestDB(t)

	if err := AdminBanDetection("server1", "user1"); err != nil {
		t.Fatalf("AdminBanDetection failed: %v", err)
	}

	err := OptInDetection("server1", "user1", false)
	if err == nil {
		t.Fatal("expected error when user tries to opt-in after admin ban, got nil")
	}
	if err != ErrAdminBanned {
		t.Errorf("expected ErrAdminBanned, got %v", err)
	}

	// Record should still exist
	if !IsDetectionOptedOut("server1", "user1") {
		t.Error("user should still be opted out after failed self opt-in")
	}
}

func TestAdminBan後にUnbanで解除できること(t *testing.T) {
	setupDetectionTestDB(t)

	if err := AdminBanDetection("server1", "user1"); err != nil {
		t.Fatalf("AdminBanDetection failed: %v", err)
	}

	if err := OptInDetection("server1", "user1", true); err != nil {
		t.Fatalf("OptInDetection with force=true failed: %v", err)
	}

	if IsDetectionOptedOut("server1", "user1") {
		t.Error("user should not be opted out after admin unban")
	}
}

func Test自己OptOutはDetectOnで解除できること(t *testing.T) {
	setupDetectionTestDB(t)

	if err := OptOutDetection("server1", "user1", SetBySelf); err != nil {
		t.Fatalf("OptOutDetection failed: %v", err)
	}

	if !IsDetectionOptedOut("server1", "user1") {
		t.Fatal("user should be opted out")
	}

	if err := OptInDetection("server1", "user1", false); err != nil {
		t.Fatalf("OptInDetection failed: %v", err)
	}

	if IsDetectionOptedOut("server1", "user1") {
		t.Error("user should not be opted out after self opt-in")
	}
}

func TestListOptOutsByServer_一覧取得(t *testing.T) {
	setupDetectionTestDB(t)

	// Create mixed opt-outs
	if err := OptOutDetection("server1", "user1", SetBySelf); err != nil {
		t.Fatalf("OptOutDetection failed: %v", err)
	}
	if err := AdminBanDetection("server1", "user2"); err != nil {
		t.Fatalf("AdminBanDetection failed: %v", err)
	}

	optOuts, err := ListOptOutsByServer("server1")
	if err != nil {
		t.Fatalf("ListOptOutsByServer failed: %v", err)
	}

	if len(optOuts) != 2 {
		t.Fatalf("expected 2 opt-outs, got %d", len(optOuts))
	}

	// Verify set_by values are present
	foundSelf := false
	foundAdmin := false
	for _, o := range optOuts {
		if o.UserID == "user1" && o.SetBy == SetBySelf {
			foundSelf = true
		}
		if o.UserID == "user2" && o.SetBy == SetByAdmin {
			foundAdmin = true
		}
	}
	if !foundSelf {
		t.Error("expected self opt-out for user1")
	}
	if !foundAdmin {
		t.Error("expected admin ban for user2")
	}
}

func TestListOptOutsByServer_空の場合(t *testing.T) {
	setupDetectionTestDB(t)

	optOuts, err := ListOptOutsByServer("server1")
	if err != nil {
		t.Fatalf("ListOptOutsByServer failed: %v", err)
	}
	if len(optOuts) != 0 {
		t.Errorf("expected 0 opt-outs, got %d", len(optOuts))
	}
}

func Testサーバー間独立性(t *testing.T) {
	setupDetectionTestDB(t)

	if err := AdminBanDetection("server1", "user1"); err != nil {
		t.Fatalf("AdminBanDetection failed: %v", err)
	}

	// user1 should not be banned in server2
	if IsDetectionOptedOut("server2", "user1") {
		t.Error("user1 should not be opted out in server2")
	}
	if IsAdminBanned("server2", "user1") {
		t.Error("user1 should not be admin banned in server2")
	}

	// user1 should be banned in server1
	if !IsDetectionOptedOut("server1", "user1") {
		t.Error("user1 should be opted out in server1")
	}
	if !IsAdminBanned("server1", "user1") {
		t.Error("user1 should be admin banned in server1")
	}
}

func Test冪等性_同じユーザーを2回ban(t *testing.T) {
	setupDetectionTestDB(t)

	if err := AdminBanDetection("server1", "user1"); err != nil {
		t.Fatalf("first AdminBanDetection failed: %v", err)
	}
	if err := AdminBanDetection("server1", "user1"); err != nil {
		t.Fatalf("second AdminBanDetection failed: %v", err)
	}

	// Should still have exactly one record
	var count int
	db.DB.Model(&model.DetectionOptOut{}).Where("server_id = ? AND user_id = ?", "server1", "user1").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 record, got %d", count)
	}
}

func TestAdminBan_selfからadminへの昇格(t *testing.T) {
	setupDetectionTestDB(t)

	// User first opts out themselves
	if err := OptOutDetection("server1", "user1", SetBySelf); err != nil {
		t.Fatalf("OptOutDetection failed: %v", err)
	}

	// Admin then bans the same user
	if err := AdminBanDetection("server1", "user1"); err != nil {
		t.Fatalf("AdminBanDetection failed: %v", err)
	}

	// Should be admin banned now
	if !IsAdminBanned("server1", "user1") {
		t.Error("user should be admin banned after upgrade from self")
	}

	// User should not be able to opt-in
	err := OptInDetection("server1", "user1", false)
	if err != ErrAdminBanned {
		t.Errorf("expected ErrAdminBanned after upgrade, got %v", err)
	}
}

func TestIsAdminBanned_selfOptOutではfalse(t *testing.T) {
	setupDetectionTestDB(t)

	if err := OptOutDetection("server1", "user1", SetBySelf); err != nil {
		t.Fatalf("OptOutDetection failed: %v", err)
	}

	if IsAdminBanned("server1", "user1") {
		t.Error("self opt-out should not be treated as admin ban")
	}
}

func TestIsAdminBanned_レコードなしではfalse(t *testing.T) {
	setupDetectionTestDB(t)

	if IsAdminBanned("server1", "user1") {
		t.Error("non-existent record should not be treated as admin ban")
	}
}

func TestDeleteOptOutByServer_adminBan含む全削除(t *testing.T) {
	setupDetectionTestDB(t)

	if err := AdminBanDetection("server1", "user1"); err != nil {
		t.Fatalf("AdminBanDetection failed: %v", err)
	}
	if err := OptOutDetection("server1", "user2", SetBySelf); err != nil {
		t.Fatalf("OptOutDetection failed: %v", err)
	}

	count, err := DeleteOptOutByServer("server1")
	if err != nil {
		t.Fatalf("DeleteOptOutByServer failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 deletions, got %d", count)
	}

	optOuts, err := ListOptOutsByServer("server1")
	if err != nil {
		t.Fatalf("ListOptOutsByServer failed: %v", err)
	}
	if len(optOuts) != 0 {
		t.Errorf("expected 0 opt-outs after server deletion, got %d", len(optOuts))
	}
}

func TestOptOutDetection_キャッシュが更新されること(t *testing.T) {
	setupDetectionTestDB(t)

	// Initially not opted out
	if IsDetectionOptedOut("server1", "user1") {
		t.Fatal("user should not be opted out initially")
	}

	if err := OptOutDetection("server1", "user1", SetBySelf); err != nil {
		t.Fatalf("OptOutDetection failed: %v", err)
	}

	// Cache should reflect opt-out
	if !IsDetectionOptedOut("server1", "user1") {
		t.Error("user should be opted out after OptOutDetection")
	}
}

func TestOptInDetection_キャッシュが更新されること(t *testing.T) {
	setupDetectionTestDB(t)

	if err := OptOutDetection("server1", "user1", SetBySelf); err != nil {
		t.Fatalf("OptOutDetection failed: %v", err)
	}

	if err := OptInDetection("server1", "user1", false); err != nil {
		t.Fatalf("OptInDetection failed: %v", err)
	}

	// Cache should reflect opt-in
	if IsDetectionOptedOut("server1", "user1") {
		t.Error("user should not be opted out after OptInDetection")
	}
}

func TestAdminBanDetection_キャッシュが更新されること(t *testing.T) {
	setupDetectionTestDB(t)

	if err := AdminBanDetection("server1", "user1"); err != nil {
		t.Fatalf("AdminBanDetection failed: %v", err)
	}

	// Cache should reflect opt-out
	if !IsDetectionOptedOut("server1", "user1") {
		t.Error("user should be opted out after AdminBanDetection")
	}
}

func TestOptInDetection_存在しないレコードでもエラーにならないこと(t *testing.T) {
	setupDetectionTestDB(t)

	if err := OptInDetection("server1", "nonexistent", false); err != nil {
		t.Fatalf("OptInDetection for nonexistent user should not error: %v", err)
	}
}
