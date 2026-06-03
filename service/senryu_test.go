package service

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/u16-io/FindSenryu4Discord/db"
	"github.com/u16-io/FindSenryu4Discord/model"
)

func setupSenryuTestDB(t *testing.T) {
	t.Helper()
	var err error
	db.DB, err = gorm.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	db.DB.AutoMigrate(&model.Senryu{}, &model.MutedChannel{}, &model.GuildChannelTypeSetting{})
	t.Cleanup(func() {
		db.DB.Close()
	})
}

func boolPtr(b bool) *bool {
	return &b
}

func TestCreateSenryu_平文のまま保存される(t *testing.T) {
	setupSenryuTestDB(t)

	spoiler := false
	created, err := CreateSenryu(model.Senryu{
		ServerID:  "server1",
		AuthorID:  "author1",
		Kamigo:    "古池や",
		Nakasichi: "蛙飛び込む",
		Simogo:    "水の音",
		Spoiler:   &spoiler,
	})
	if err != nil {
		t.Fatalf("CreateSenryu failed: %v", err)
	}

	var raw model.Senryu
	if err := db.DB.Where("id = ?", created.ID).First(&raw).Error; err != nil {
		t.Fatalf("failed to read raw senryu: %v", err)
	}

	if raw.Kamigo != "古池や" {
		t.Errorf("Kamigo = %q, want %q", raw.Kamigo, "古池や")
	}
	if raw.Nakasichi != "蛙飛び込む" {
		t.Errorf("Nakasichi = %q, want %q", raw.Nakasichi, "蛙飛び込む")
	}
	if raw.Simogo != "水の音" {
		t.Errorf("Simogo = %q, want %q", raw.Simogo, "水の音")
	}
	if created.ID == 0 {
		t.Error("created ID should be non-zero")
	}
}

func TestDeleteSenryu_削除できる(t *testing.T) {
	setupSenryuTestDB(t)

	spoiler := false
	created, err := CreateSenryu(model.Senryu{
		ServerID:  "server1",
		AuthorID:  "author1",
		Kamigo:    "削除テスト",
		Nakasichi: "普通の句でも",
		Simogo:    "消せるはず",
		Spoiler:   &spoiler,
	})
	if err != nil {
		t.Fatalf("CreateSenryu failed: %v", err)
	}

	if err := DeleteSenryu(created.ID, "server1"); err != nil {
		t.Fatalf("DeleteSenryu failed: %v", err)
	}

	_, err = GetSenryuByID(created.ID, "server1")
	if err != ErrSenryuNotFound {
		t.Errorf("expected ErrSenryuNotFound, got %v", err)
	}
}

func TestGetRanking_集計できる(t *testing.T) {
	setupSenryuTestDB(t)

	spoiler := false
	for i := 0; i < 3; i++ {
		_, err := CreateSenryu(model.Senryu{
			ServerID:  "server1",
			AuthorID:  "author1",
			Kamigo:    "ランキング",
			Nakasichi: "テストの句です",
			Simogo:    "数えよう",
			Spoiler:   &spoiler,
		})
		if err != nil {
			t.Fatalf("CreateSenryu failed: %v", err)
		}
	}

	ranks, err := GetRanking("server1")
	if err != nil {
		t.Fatalf("GetRanking failed: %v", err)
	}

	if len(ranks) != 1 {
		t.Fatalf("expected 1 rank entry, got %d", len(ranks))
	}
	if ranks[0].Count != 3 {
		t.Errorf("expected count 3, got %d", ranks[0].Count)
	}
	if ranks[0].AuthorId != "author1" {
		t.Errorf("expected author1, got %s", ranks[0].AuthorId)
	}
}

func seedSenryus(t *testing.T, serverID, authorID string, count int) {
	t.Helper()
	f := false
	for i := 0; i < count; i++ {
		s := model.Senryu{
			ServerID:  serverID,
			AuthorID:  authorID,
			Kamigo:    "上の句",
			Nakasichi: "中の句",
			Simogo:    "下の句",
			Spoiler:   &f,
		}
		if err := db.DB.Create(&s).Error; err != nil {
			t.Fatalf("failed to seed senryu: %v", err)
		}
	}
}

func TestGetSenryusByAuthorPaged_ページネーション(t *testing.T) {
	setupSenryuTestDB(t)
	seedSenryus(t, "guild1", "user1", 30)

	// 1ページ目（25件）
	page1, err := GetSenryusByAuthorPaged("guild1", "user1", 25, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page1) != 25 {
		t.Errorf("expected 25 senryus, got %d", len(page1))
	}

	// 2ページ目（残り5件）
	page2, err := GetSenryusByAuthorPaged("guild1", "user1", 25, 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page2) != 5 {
		t.Errorf("expected 5 senryus, got %d", len(page2))
	}

	// IDの重複がないこと
	ids := make(map[int]bool)
	for _, s := range page1 {
		ids[s.ID] = true
	}
	for _, s := range page2 {
		if ids[s.ID] {
			t.Errorf("duplicate senryu ID %d across pages", s.ID)
		}
	}
}

func TestGetSenryusByAuthorPaged_降順(t *testing.T) {
	setupSenryuTestDB(t)
	seedSenryus(t, "guild1", "user1", 5)

	results, err := GetSenryusByAuthorPaged("guild1", "user1", 25, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 1; i < len(results); i++ {
		if results[i].ID >= results[i-1].ID {
			t.Errorf("expected descending order: ID %d >= %d", results[i].ID, results[i-1].ID)
		}
	}
}

func TestGetSenryusByAuthorPaged_該当なし(t *testing.T) {
	setupSenryuTestDB(t)

	results, err := GetSenryusByAuthorPaged("guild1", "user1", 25, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 senryus, got %d", len(results))
	}
}

func TestGetSenryusByAuthorPaged_別サーバーの川柳は含まない(t *testing.T) {
	setupSenryuTestDB(t)
	seedSenryus(t, "guild1", "user1", 5)
	seedSenryus(t, "guild2", "user1", 3)

	results, err := GetSenryusByAuthorPaged("guild1", "user1", 25, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5 senryus for guild1, got %d", len(results))
	}
}

func TestGetSenryusByAuthorPaged_別ユーザーの川柳は含まない(t *testing.T) {
	setupSenryuTestDB(t)
	seedSenryus(t, "guild1", "user1", 5)
	seedSenryus(t, "guild1", "user2", 3)

	results, err := GetSenryusByAuthorPaged("guild1", "user1", 25, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5 senryus for user1, got %d", len(results))
	}
}

func TestGetSenryusByAuthorPaged_offset超過で空(t *testing.T) {
	setupSenryuTestDB(t)
	seedSenryus(t, "guild1", "user1", 3)

	results, err := GetSenryusByAuthorPaged("guild1", "user1", 25, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 senryus with large offset, got %d", len(results))
	}
}

func TestCountSenryusByAuthor_正常(t *testing.T) {
	setupSenryuTestDB(t)
	seedSenryus(t, "guild1", "user1", 30)

	count, err := CountSenryusByAuthor("guild1", "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 30 {
		t.Errorf("expected 30, got %d", count)
	}
}

func TestCountSenryusByAuthor_該当なし(t *testing.T) {
	setupSenryuTestDB(t)

	count, err := CountSenryusByAuthor("guild1", "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestCountSenryusByAuthor_別サーバーは含まない(t *testing.T) {
	setupSenryuTestDB(t)
	seedSenryus(t, "guild1", "user1", 5)
	seedSenryus(t, "guild2", "user1", 3)

	count, err := CountSenryusByAuthor("guild1", "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5, got %d", count)
	}
}

func TestCountSenryusByAuthor_別ユーザーは含まない(t *testing.T) {
	setupSenryuTestDB(t)
	seedSenryus(t, "guild1", "user1", 5)
	seedSenryus(t, "guild1", "user2", 3)

	count, err := CountSenryusByAuthor("guild1", "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5, got %d", count)
	}
}

func TestGetServerStats_正常系(t *testing.T) {
	setupSenryuTestDB(t)
	seedSenryus(t, "guild1", "user1", 3)
	seedSenryus(t, "guild1", "user2", 2)

	stats, err := GetServerStats("guild1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalSenryus != 5 {
		t.Errorf("TotalSenryus = %d, want 5", stats.TotalSenryus)
	}
	if stats.UniqueAuthors != 2 {
		t.Errorf("UniqueAuthors = %d, want 2", stats.UniqueAuthors)
	}
}

func TestGetServerStats_川柳が0件(t *testing.T) {
	setupSenryuTestDB(t)

	stats, err := GetServerStats("guild1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalSenryus != 0 {
		t.Errorf("TotalSenryus = %d, want 0", stats.TotalSenryus)
	}
	if stats.UniqueAuthors != 0 {
		t.Errorf("UniqueAuthors = %d, want 0", stats.UniqueAuthors)
	}
}

func TestGetServerStats_別サーバーの川柳は含まない(t *testing.T) {
	setupSenryuTestDB(t)
	seedSenryus(t, "guild1", "user1", 3)
	seedSenryus(t, "guild2", "user2", 5)

	stats, err := GetServerStats("guild1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalSenryus != 3 {
		t.Errorf("TotalSenryus = %d, want 3", stats.TotalSenryus)
	}
	if stats.UniqueAuthors != 1 {
		t.Errorf("UniqueAuthors = %d, want 1", stats.UniqueAuthors)
	}
}

func TestGetServerStats_同一ユーザーが複数句(t *testing.T) {
	setupSenryuTestDB(t)
	seedSenryus(t, "guild1", "user1", 7)

	stats, err := GetServerStats("guild1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalSenryus != 7 {
		t.Errorf("TotalSenryus = %d, want 7", stats.TotalSenryus)
	}
	if stats.UniqueAuthors != 1 {
		t.Errorf("UniqueAuthors = %d, want 1", stats.UniqueAuthors)
	}
}

func TestGetLastSenryu_最後の川柳を返す(t *testing.T) {
	setupSenryuTestDB(t)

	db.DB.Create(&model.Senryu{
		ServerID: "guild1", AuthorID: "user1",
		Kamigo: "古池や", Nakasichi: "蛙飛び込む", Simogo: "水の音",
		Spoiler: boolPtr(false),
	})
	db.DB.Create(&model.Senryu{
		ServerID: "guild1", AuthorID: "user2",
		Kamigo: "柿くへば", Nakasichi: "鐘が鳴るなり", Simogo: "法隆寺",
		Spoiler: boolPtr(false),
	})

	got, err := GetLastSenryu("guild1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AuthorID != "user2" {
		t.Errorf("AuthorID = %q, want %q", got.AuthorID, "user2")
	}
	if got.Kamigo != "柿くへば" {
		t.Errorf("Kamigo = %q, want %q", got.Kamigo, "柿くへば")
	}
	if got.Nakasichi != "鐘が鳴るなり" {
		t.Errorf("Nakasichi = %q, want %q", got.Nakasichi, "鐘が鳴るなり")
	}
	if got.Simogo != "法隆寺" {
		t.Errorf("Simogo = %q, want %q", got.Simogo, "法隆寺")
	}
}

func TestGetLastSenryu_川柳が存在しない場合(t *testing.T) {
	setupSenryuTestDB(t)

	_, err := GetLastSenryu("guild1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrSenryuNotFound) {
		t.Errorf("error = %v, want ErrSenryuNotFound", err)
	}
}

func TestGetLastSenryu_サーバーごとに独立(t *testing.T) {
	setupSenryuTestDB(t)

	db.DB.Create(&model.Senryu{
		ServerID: "guild1", AuthorID: "user1",
		Kamigo: "古池や", Nakasichi: "蛙飛び込む", Simogo: "水の音",
		Spoiler: boolPtr(false),
	})
	db.DB.Create(&model.Senryu{
		ServerID: "guild2", AuthorID: "user2",
		Kamigo: "柿くへば", Nakasichi: "鐘が鳴るなり", Simogo: "法隆寺",
		Spoiler: boolPtr(false),
	})

	got, err := GetLastSenryu("guild1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AuthorID != "user1" {
		t.Errorf("AuthorID = %q, want %q", got.AuthorID, "user1")
	}
	if got.ServerID != "guild1" {
		t.Errorf("ServerID = %q, want %q", got.ServerID, "guild1")
	}
}

func TestGetLastSenryu_スポイラー付き川柳(t *testing.T) {
	setupSenryuTestDB(t)

	db.DB.Create(&model.Senryu{
		ServerID: "guild1", AuthorID: "user1",
		Kamigo: "秘密の", Nakasichi: "内容が含まれる", Simogo: "川柳だ",
		Spoiler: boolPtr(true),
	})

	got, err := GetLastSenryu("guild1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Spoiler == nil || !*got.Spoiler {
		t.Error("Spoiler should be true")
	}
}

func TestGetLastSenryu_スポイラーなし川柳(t *testing.T) {
	setupSenryuTestDB(t)

	db.DB.Create(&model.Senryu{
		ServerID: "guild1", AuthorID: "user1",
		Kamigo: "古池や", Nakasichi: "蛙飛び込む", Simogo: "水の音",
		Spoiler: boolPtr(false),
	})

	got, err := GetLastSenryu("guild1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Spoiler == nil || *got.Spoiler {
		t.Error("Spoiler should be false")
	}
}
