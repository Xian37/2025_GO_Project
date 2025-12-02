package errors

import (
	"errors"
	"fmt"
)

// 定義錯誤類型
var (
	// ErrRoomNotFound 房間不存在
	ErrRoomNotFound = errors.New("room not found")

	// ErrClientNotFound 客戶端不存在
	ErrClientNotFound = errors.New("client not found")

	// ErrInvalidPassword 密碼錯誤
	ErrInvalidPassword = errors.New("invalid password")

	// ErrRateLimitExceeded 超過訊息限制
	ErrRateLimitExceeded = errors.New("rate limit exceeded")

	// ErrMessageTooLarge 訊息過大
	ErrMessageTooLarge = errors.New("message too large")

	// ErrInvalidMessage 無效訊息
	ErrInvalidMessage = errors.New("invalid message")

	// ErrConnectionClosed 連線已關閉
	ErrConnectionClosed = errors.New("connection closed")

	// ErrStorageFailure 儲存失敗
	ErrStorageFailure = errors.New("storage operation failed")
)

// ChatError 聊天室自訂錯誤
type ChatError struct {
	Op      string // 操作名稱
	Err     error  // 原始錯誤
	Code    int    // 錯誤代碼
	Message string // 錯誤訊息
}

func (e *ChatError) Error() string {
	if e.Op != "" {
		return fmt.Sprintf("%s: %s", e.Op, e.Message)
	}
	return e.Message
}

func (e *ChatError) Unwrap() error {
	return e.Err
}

// NewChatError 建立新的聊天室錯誤
func NewChatError(op string, err error, code int, message string) *ChatError {
	return &ChatError{
		Op:      op,
		Err:     err,
		Code:    code,
		Message: message,
	}
}

// IsConnectionError 判斷是否為連線錯誤
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrConnectionClosed)
}

// IsRateLimitError 判斷是否為限流錯誤
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrRateLimitExceeded)
}
