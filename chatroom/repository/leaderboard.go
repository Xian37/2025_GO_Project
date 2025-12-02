package repository

import (
	"chatroom/models"
	"encoding/json"
	"os"
	"sort"
	"sync"
)

// LeaderboardRepository 排行榜資料存取介面
type LeaderboardRepository interface {
	Load() ([]models.GameScore, error)
	Save(scores []models.GameScore) error
	Add(score models.GameScore) error
	GetTop(n int) ([]models.GameScore, error)
	GetAll() []models.GameScore
	Clear() error
}

// FileLeaderboardRepository 檔案型排行榜儲存
type FileLeaderboardRepository struct {
	mu       sync.RWMutex
	filePath string
	scores   []models.GameScore
}

// NewFileLeaderboardRepository 創建新的檔案型排行榜儲存
func NewFileLeaderboardRepository(filePath string) *FileLeaderboardRepository {
	repo := &FileLeaderboardRepository{
		filePath: filePath,
		scores:   make([]models.GameScore, 0),
	}

	// 嘗試載入現有資料
	repo.Load()

	return repo
}

// Load 從檔案載入排行榜
func (r *FileLeaderboardRepository) Load() ([]models.GameScore, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	file, err := os.ReadFile(r.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 檔案不存在，返回空列表
			r.scores = make([]models.GameScore, 0)
			return r.scores, nil
		}
		return nil, err
	}

	err = json.Unmarshal(file, &r.scores)
	if err != nil {
		r.scores = make([]models.GameScore, 0)
		return nil, err
	}

	return r.scores, nil
}

// Save 儲存排行榜到檔案
func (r *FileLeaderboardRepository) Save(scores []models.GameScore) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.scores = scores

	file, err := json.MarshalIndent(r.scores, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.filePath, file, 0644)
}

// Add 新增分數並排序
func (r *FileLeaderboardRepository) Add(score models.GameScore) error {
	r.mu.Lock()
	r.scores = append(r.scores, score)

	// 排序：嘗試次數少的優先，次數相同則時間短的優先
	sort.Slice(r.scores, func(i, j int) bool {
		if r.scores[i].Tries != r.scores[j].Tries {
			return r.scores[i].Tries < r.scores[j].Tries
		}
		return r.scores[i].Time < r.scores[j].Time
	})

	// 只保留前 10 名
	if len(r.scores) > 10 {
		r.scores = r.scores[:10]
	}
	r.mu.Unlock()

	return r.Save(r.scores)
}

// GetTop 獲取前 N 名
func (r *FileLeaderboardRepository) GetTop(n int) ([]models.GameScore, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if n > len(r.scores) {
		n = len(r.scores)
	}

	result := make([]models.GameScore, n)
	copy(result, r.scores[:n])

	return result, nil
}

// Clear 清空排行榜
func (r *FileLeaderboardRepository) Clear() error {
	return r.Save(make([]models.GameScore, 0))
}

// GetAll 獲取所有分數
func (r *FileLeaderboardRepository) GetAll() []models.GameScore {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]models.GameScore, len(r.scores))
	copy(result, r.scores)
	return result
}
