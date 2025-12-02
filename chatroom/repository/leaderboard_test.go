package repository

import (
	"chatroom/models"
	"os"
	"testing"
)

func TestFileLeaderboardRepository(t *testing.T) {
	// 使用臨時檔案
	tmpFile := "test_leaderboard.json"
	defer os.Remove(tmpFile)

	repo := NewFileLeaderboardRepository(tmpFile)

	t.Run("Add and GetTop", func(t *testing.T) {
		// 添加測試分數
		scores := []models.GameScore{
			{Nickname: "Player1", Tries: 5, Time: 30},
			{Nickname: "Player2", Tries: 3, Time: 20},
			{Nickname: "Player3", Tries: 4, Time: 25},
		}

		for _, score := range scores {
			err := repo.Add(score)
			if err != nil {
				t.Fatalf("Failed to add score: %v", err)
			}
		}

		// 獲取前 2 名
		top, err := repo.GetTop(2)
		if err != nil {
			t.Fatalf("Failed to get top scores: %v", err)
		}

		if len(top) != 2 {
			t.Errorf("Expected 2 scores, got %d", len(top))
		}

		// 檢查排序 (Tries 少的優先)
		if top[0].Nickname != "Player2" {
			t.Errorf("Expected Player2 first, got %s", top[0].Nickname)
		}
	})

	t.Run("Load and Save", func(t *testing.T) {
		// 重新載入
		newRepo := NewFileLeaderboardRepository(tmpFile)
		scores := newRepo.GetAll()

		if len(scores) != 3 {
			t.Errorf("Expected 3 scores after reload, got %d", len(scores))
		}
	})

	t.Run("Clear", func(t *testing.T) {
		err := repo.Clear()
		if err != nil {
			t.Fatalf("Failed to clear: %v", err)
		}

		scores := repo.GetAll()
		if len(scores) != 0 {
			t.Errorf("Expected 0 scores after clear, got %d", len(scores))
		}
	})
}

func TestLeaderboardSorting(t *testing.T) {
	tmpFile := "test_sort.json"
	defer os.Remove(tmpFile)

	repo := NewFileLeaderboardRepository(tmpFile)

	// 添加相同 Tries 但不同 Time 的分數
	repo.Add(models.GameScore{Nickname: "Fast", Tries: 5, Time: 10})
	repo.Add(models.GameScore{Nickname: "Slow", Tries: 5, Time: 20})

	top, _ := repo.GetTop(2)

	if top[0].Nickname != "Fast" {
		t.Errorf("Expected Fast first (shorter time), got %s", top[0].Nickname)
	}
}

func TestMaxLeaderboardSize(t *testing.T) {
	tmpFile := "test_max.json"
	defer os.Remove(tmpFile)

	repo := NewFileLeaderboardRepository(tmpFile)

	// 添加 15 個分數
	for i := 1; i <= 15; i++ {
		repo.Add(models.GameScore{
			Nickname: "Player" + string(rune(i)),
			Tries:    i,
			Time:     i * 10,
		})
	}

	all := repo.GetAll()
	if len(all) != 10 {
		t.Errorf("Expected max 10 scores, got %d", len(all))
	}
}
