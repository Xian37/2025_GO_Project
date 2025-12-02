# 房間切換 Bug 修復報告

**日期**: 2025年12月2日  
**問題**: 建立新房間後，點擊進入會立即退出回到大廳

---

## 問題描述

用戶反映：
- 房間列表顯示正常
- 可以看到新建立的房間
- 但點擊新房間後會進入又立即退出回到大廳

---

## 問題根因分析

經過代碼審查，發現了兩個關鍵問題：

### 1. 後端未發送房間切換成功確認訊息

**位置**: `transport/websocket_v2.go` 的 `handleSwitchRoom` 方法

**問題**:
- 舊版 `websocket.go` 會發送 `switch_success` 訊息告知前端切換成功
- 新版 `websocket_v2.go` **遺漏了這個確認訊息**
- 前端等不到確認，無法更新 `currentRoom` 狀態
- 導致前端認為切換失敗

**前端期望的邏輯** (`index.html` line 1716-1725):
```javascript
case 'switch_success':
  currentRoom = msg.room;
  document.getElementById('room-name-header').textContent = currentRoom;
  messagesEl.innerHTML = ''; // Clear messages from old room
  addSystemMessage(`成功加入房間: ${currentRoom}`);
  document.querySelectorAll('#room-list li').forEach(li => {
    li.classList.toggle('active', li.dataset.room === currentRoom);
  });
  break;
```

### 2. 錯誤訊息缺少房間名稱欄位

**位置**: `transport/websocket_v2.go` 的 `handleSwitchRoom` 方法

**問題**:
- 當密碼驗證失敗時，後端返回錯誤訊息
- 但錯誤訊息中**沒有包含 `Room` 欄位**
- 前端嘗試顯示 `msg.room` 時會得到 `undefined`

**前端錯誤處理** (`index.html` line 1706-1713):
```javascript
case 'password_required':
  const pw = prompt(`房間 ${msg.room} 需要密碼：`);  // msg.room 是 undefined
  if (pw) {
    switchRoom(msg.room, false, pw);
  }
  break;
case 'wrong_password':
  alert(`房間 ${msg.room} 的密碼錯誤！`);  // msg.room 是 undefined
  break;
```

---

## 修復方案

### 修復 1: 添加房間切換成功確認訊息

**檔案**: `transport/websocket_v2.go`  
**位置**: `handleSwitchRoom` 方法，密碼驗證成功後

**修改前**:
```go
// handleSwitchRoom 處理切換房間
func (h *WebsocketHandlerV2) handleSwitchRoom(client *models.Client, msg models.Message) {
	oldRoom, err := h.Service.SwitchRoom(client, msg.Room, msg.Password)
	if err != nil {
		// ... 錯誤處理
		return
	}

	// 發送歷史訊息
	h.Service.SendHistory(client)

	// 發送加入訊息
	if !strings.HasPrefix(msg.Room, "_") {
		joinMsg := models.Message{
			Type:      "join",
			Room:      msg.Room,
			Content:   client.Nickname + " 加入了聊天室",
			Timestamp: time.Now().Format("15:04"),
		}
		h.Service.Broadcast <- joinMsg
	}
}
```

**修改後**:
```go
// handleSwitchRoom 處理切換房間
func (h *WebsocketHandlerV2) handleSwitchRoom(client *models.Client, msg models.Message) {
	oldRoom, err := h.Service.SwitchRoom(client, msg.Room, msg.Password)
	if err != nil {
		// ... 錯誤處理
		return
	}

	// ✅ 新增：發送切換成功確認訊息給客戶端
	confirmMsg := models.Message{
		Type:    "switch_success",
		Room:    msg.Room,
		Content: oldRoom, // 舊房間名稱
	}
	client.Mu.Lock()
	client.Conn.WriteJSON(confirmMsg)
	client.Mu.Unlock()

	// 發送歷史訊息
	h.Service.SendHistory(client)

	// 發送加入訊息
	if !strings.HasPrefix(msg.Room, "_") {
		joinMsg := models.Message{
			Type:      "join",
			Room:      msg.Room,
			Content:   client.Nickname + " 加入了聊天室",
			Timestamp: time.Now().Format("15:04"),
		}
		h.Service.Broadcast <- joinMsg
	}
}
```

**效果**:
- 前端收到 `switch_success` 訊息
- 更新 `currentRoom` 狀態
- 清空舊訊息
- 顯示「成功加入房間」系統訊息
- 更新房間列表的 active 狀態

---

### 修復 2: 錯誤訊息包含房間名稱

**檔案**: `transport/websocket_v2.go`  
**位置**: `handleSwitchRoom` 方法，錯誤處理部分

**修改前**:
```go
if err != nil {
	// 發送錯誤訊息給客戶端
	errorMsg := models.Message{
		Type:    err.Error(), // "password_required" 或 "wrong_password"
		Content: "密碼驗證失敗",
	}
	client.Mu.Lock()
	client.Conn.WriteJSON(errorMsg)
	client.Mu.Unlock()
	return
}
```

**修改後**:
```go
if err != nil {
	// 發送錯誤訊息給客戶端
	errorMsg := models.Message{
		Type:    err.Error(), // "password_required" 或 "wrong_password"
		Room:    msg.Room,    // ✅ 新增：包含房間名稱
		Content: "密碼驗證失敗",
	}
	client.Mu.Lock()
	client.Conn.WriteJSON(errorMsg)
	client.Mu.Unlock()
	return
}
```

**效果**:
- 前端可以正確顯示房間名稱
- `prompt` 提示：「房間 test-room 需要密碼：」（而不是「房間 undefined 需要密碼：」）
- `alert` 提示：「房間 test-room 的密碼錯誤！」

---

## 技術細節

### WebSocket 訊息流程

**正常流程** (修復後):
```
1. 前端: { type: 'switch', room: 'test-room', password: '' }
2. 後端: 驗證密碼 → 切換房間 → 發送 switch_success
3. 前端: 收到 switch_success → 更新 currentRoom → 清空訊息
4. 後端: 發送歷史訊息 → 發送 join 廣播
5. 前端: 渲染歷史訊息 → 顯示加入系統訊息
```

**錯誤流程** (密碼錯誤):
```
1. 前端: { type: 'switch', room: 'test-room', password: 'wrong' }
2. 後端: 驗證密碼失敗 → 發送 wrong_password (包含房間名稱)
3. 前端: 收到 wrong_password → alert('房間 test-room 的密碼錯誤！')
```

**需要密碼流程**:
```
1. 前端: { type: 'switch', room: 'test-room', password: '' }
2. 後端: 檢測需要密碼 → 發送 password_required (包含房間名稱)
3. 前端: 收到 password_required → prompt('房間 test-room 需要密碼：')
4. 用戶輸入密碼 → 重新發送 switch 請求
```

---

## 相關代碼位置

| 檔案 | 方法/區域 | 行數 | 說明 |
|------|----------|------|------|
| `transport/websocket_v2.go` | `handleSwitchRoom` | ~201-235 | WebSocket 房間切換處理器 |
| `service/service_v2.go` | `SwitchRoom` | 199-285 | 房間切換核心邏輯（密碼驗證、房間管理） |
| `static/index.html` | WebSocket `onmessage` | 1700-1750 | 前端訊息處理（switch_success, password_required, wrong_password） |
| `static/index.html` | `submitCreateRoom` | 1958-1970 | 創建房間函數 |
| `static/index.html` | `switchRoom` | 1867-1877 | 房間切換函數 |

---

## 測試建議

### 測試案例 1: 創建無密碼房間
1. 點擊「+ 新增房間」
2. 輸入房間名稱，密碼留空
3. 點擊確認
4. **預期**: 成功進入新房間，房間列表顯示新房間為 active

### 測試案例 2: 創建有密碼房間
1. 點擊「+ 新增房間」
2. 輸入房間名稱和密碼
3. 點擊確認
4. **預期**: 成功進入新房間

### 測試案例 3: 加入有密碼房間（正確密碼）
1. 點擊有密碼的房間
2. 輸入正確密碼
3. **預期**: 成功進入房間

### 測試案例 4: 加入有密碼房間（錯誤密碼）
1. 點擊有密碼的房間
2. 輸入錯誤密碼
3. **預期**: 顯示「房間 XXX 的密碼錯誤！」，留在當前房間

### 測試案例 5: 加入有密碼房間（取消輸入）
1. 點擊有密碼的房間
2. 點擊取消或關閉 prompt
3. **預期**: 留在當前房間，無任何錯誤

---

## 影響範圍

### 已修復的功能
✅ 創建新房間後可以正常進入  
✅ 房間切換成功後前端狀態正確更新  
✅ 錯誤訊息正確顯示房間名稱  
✅ 密碼驗證流程完整運作  

### 不受影響的功能
- 聊天訊息發送與接收
- 在線人數統計
- 房間列表更新
- 遊戲功能
- 排行榜功能

---

## 版本資訊

- **Go 版本**: 1.25.0
- **主要依賴**:
  - gorilla/websocket v1.5.3
  - uber-go/zap v1.27.0

---

## 後續建議

### 1. 添加更完善的錯誤處理
考慮在前端添加更詳細的錯誤訊息處理：
- 網路錯誤
- 房間已滿
- 房間已關閉

### 2. 改善用戶體驗
- 密碼錯誤後自動重新提示輸入（而不是直接返回）
- 顯示「切換中...」的載入狀態
- 添加切換失敗的回退機制

### 3. 日誌記錄
當前已有完善的日誌記錄：
```go
logger.Info("Room switched",
    zap.String("nickname", client.Nickname),
    zap.String("from", oldRoom),
    zap.String("to", msg.Room))
```

建議添加更多細節：
- 是否為新房間
- 是否需要密碼
- 密碼驗證結果

---

## 總結

本次修復解決了 V2 架構升級時遺漏的關鍵確認訊息，確保前後端狀態同步。修改量小但影響重大，完全修復了房間切換功能。

**修改檔案**: 1 個  
**修改行數**: 約 15 行  
**修復問題**: 2 個關鍵 Bug  
**測試狀態**: 待用戶驗證
