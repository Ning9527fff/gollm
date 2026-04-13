package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Ning9527fff/gollm/llm"
	"github.com/google/uuid"
)

// Manager 会话管理器
type Manager struct {
	config  Config
	storage EventStorage
	mu      sync.RWMutex // 保护 sessions 的并发访问
	sessions map[string]*Session // 内存缓存（sessionID -> Session）

	// 可选的快照存储（后续实现）
	snapshotStorage SnapshotStorage
}

// NewManager 创建会话管理器
func NewManager(config Config) (*Manager, error) {
	// 1. 创建事件存储
	storage, err := NewEventStorage(config.Storage)
	if err != nil {
		return nil, fmt.Errorf("failed to create event storage: %w", err)
	}

	// 2. 创建管理器实例
	mgr := &Manager{
		config:   config,
		storage:  storage,
		sessions: make(map[string]*Session),
	}

	return mgr, nil
}

// Close 关闭管理器，释放资源
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 清空内存缓存
	m.sessions = make(map[string]*Session)

	// 关闭存储连接
	if m.storage != nil {
		if err := m.storage.Close(); err != nil {
			return fmt.Errorf("failed to close storage: %w", err)
		}
	}

	// 关闭快照存储（如果有）
	if m.snapshotStorage != nil {
		if err := m.snapshotStorage.Close(); err != nil {
			return fmt.Errorf("failed to close snapshot storage: %w", err)
		}
	}

	return nil
}

// Exists 检查会话是否存在
func (m *Manager) Exists(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 先检查内存缓存
	if _, ok := m.sessions[sessionID]; ok {
		return true
	}

	// 从存储中检查（通过尝试获取事件）
	ctx := context.Background()
	events, err := m.storage.GetBySessionID(ctx, sessionID, 0)
	return err == nil && len(events) > 0
}

// generateEventID 生成唯一的事件 ID
func (m *Manager) generateEventID() string {
	return fmt.Sprintf("evt_%s", uuid.New().String()[:8])
}

// recordEvent 记录事件到存储（内部方法）
func (m *Manager) recordEvent(ctx context.Context, sessionID string, eventType EventType, data map[string]interface{}) error {
	if !m.config.EnableEventLog {
		return nil // 事件日志未启用，跳过
	}

	// 获取会话的当前事件计数
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return err
	}

	// 创建事件
	event := &Event{
		ID:        m.generateEventID(),
		SessionID: sessionID,
		Type:      eventType,
		Timestamp: time.Now(),
		Index:     session.EventCount,
		Data:      data,
		Metadata:  make(map[string]string),
	}

	// 写入存储
	if err := m.storage.Append(ctx, event); err != nil {
		return fmt.Errorf("failed to append event: %w", err)
	}

	// 更新会话的事件计数
	session.EventCount++

	// 检查是否需要创建快照
	if m.config.EnableSnapshot && m.config.SnapshotInterval > 0 {
		if session.EventCount%m.config.SnapshotInterval == 0 {
			// TODO: 触发快照创建（后续实现）
		}
	}

	return nil
}

// getSessionFromCache 从内存缓存获取会话（不存在则从存储恢复）
func (m *Manager) getSessionFromCache(sessionID string) (*Session, error) {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()

	if ok {
		return session, nil
	}

	// 缓存未命中，从存储中恢复
	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查（避免并发重复加载）
	if session, ok := m.sessions[sessionID]; ok {
		return session, nil
	}

	// 从事件流重建会话
	ctx := context.Background()
	session, err := m.storage.Replay(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to replay session: %w", err)
	}

	// 加载到缓存
	m.sessions[sessionID] = session

	return session, nil
}

// updateSessionCache 更新内存缓存中的会话
func (m *Manager) updateSessionCache(session *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session.UpdatedAt = time.Now()
	m.sessions[session.ID] = session
}

// removeSessionFromCache 从内存缓存中移除会话
func (m *Manager) removeSessionFromCache(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, sessionID)
}

// ========== CRUD 操作 ==========

// Create 创建新会话
func (m *Manager) Create(sessionID string, metadata map[string]string) (*Session, error) {
	// 1. 检查会话是否已存在
	if m.Exists(sessionID) {
		return nil, fmt.Errorf("session already exists: %s", sessionID)
	}

	// 2. 创建会话对象
	now := time.Now()
	session := &Session{
		ID:           sessionID,
		Messages:     []llm.Message{},
		Metadata:     metadata,
		CreatedAt:    now,
		UpdatedAt:    now,
		EventCount:   0,
		TotalTokens:  0,
		MessageCount: 0,
	}

	if session.Metadata == nil {
		session.Metadata = make(map[string]string)
	}

	// 3. 记录 session_created 事件
	ctx := context.Background()
	if err := m.recordEvent(ctx, sessionID, EventSessionCreated, map[string]interface{}{
		"metadata":   metadata,
		"created_at": now.Format(time.RFC3339),
	}); err != nil {
		return nil, fmt.Errorf("failed to record session created event: %w", err)
	}

	// 4. 更新到缓存
	m.updateSessionCache(session)

	return session, nil
}

// Get 获取会话
func (m *Manager) Get(sessionID string) (*Session, error) {
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return session, nil
}

// Delete 删除会话
func (m *Manager) Delete(sessionID string) error {
	// 1. 检查会话是否存在
	if !m.Exists(sessionID) {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// 2. 记录 session_closed 事件
	ctx := context.Background()
	if err := m.recordEvent(ctx, sessionID, EventSessionClosed, map[string]interface{}{
		"closed_at": time.Now().Format(time.RFC3339),
	}); err != nil {
		// 即使记录失败，也继续删除
		fmt.Printf("warning: failed to record session closed event: %v\n", err)
	}

	// 3. 从存储中删除
	if err := m.storage.Delete(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to delete session from storage: %w", err)
	}

	// 4. 从缓存中移除
	m.removeSessionFromCache(sessionID)

	return nil
}

// Touch 更新会话的最后访问时间（用于 TTL 管理）
func (m *Manager) Touch(sessionID string) error {
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.UpdatedAt = time.Now()
	m.updateSessionCache(session)

	return nil
}

// List 列出会话（带过滤）
func (m *Manager) List(filter SessionFilter) ([]*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Session

	// 遍历缓存中的会话
	for _, session := range m.sessions {
		// 应用过滤器
		if !m.matchFilter(session, filter) {
			continue
		}

		result = append(result, session)
	}

	// 应用分页
	start := filter.Offset
	end := filter.Offset + filter.Limit

	if start > len(result) {
		return []*Session{}, nil
	}

	if filter.Limit > 0 && end < len(result) {
		result = result[start:end]
	} else if start > 0 {
		result = result[start:]
	}

	return result, nil
}

// matchFilter 检查会话是否匹配过滤条件
func (m *Manager) matchFilter(session *Session, filter SessionFilter) bool {
	// 按创建时间过滤
	if !filter.CreatedAfter.IsZero() && session.CreatedAt.Before(filter.CreatedAfter) {
		return false
	}

	if !filter.CreatedBefore.IsZero() && session.CreatedAt.After(filter.CreatedBefore) {
		return false
	}

	// 按元数据过滤
	if len(filter.Metadata) > 0 {
		for key, value := range filter.Metadata {
			if session.Metadata[key] != value {
				return false
			}
		}
	}

	return true
}

// GC 清理过期会话（根据 TTL）
func (m *Manager) GC(maxAge time.Duration) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if maxAge == 0 {
		maxAge = m.config.TTL
	}

	if maxAge == 0 {
		return 0, nil // TTL 未设置，不执行清理
	}

	ctx := context.Background()
	cutoffTime := time.Now().Add(-maxAge)
	deletedCount := 0

	// 查找过期会话
	for sessionID, session := range m.sessions {
		if session.UpdatedAt.Before(cutoffTime) {
			// 删除存储
			if err := m.storage.Delete(ctx, sessionID); err != nil {
				fmt.Printf("warning: failed to delete expired session %s: %v\n", sessionID, err)
				continue
			}

			// 从缓存移除
			delete(m.sessions, sessionID)
			deletedCount++
		}
	}

	return deletedCount, nil
}

// ========== 消息操作 ==========

// AppendMessage 添加消息到会话
func (m *Manager) AppendMessage(sessionID string, message llm.Message) error {
	// 1. 获取会话
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// 2. 检查消息数量限制
	if m.config.MaxMessageHistory > 0 && len(session.Messages) >= m.config.MaxMessageHistory {
		// TODO: 触发自动压缩（后续实现）
		if !m.config.AutoCompress {
			return fmt.Errorf("message history limit exceeded: %d", m.config.MaxMessageHistory)
		}
	}

	// 3. 添加消息到会话
	session.Messages = append(session.Messages, message)
	session.MessageCount = len(session.Messages)

	// 4. 记录事件
	ctx := context.Background()
	eventType := EventUserMessage
	if message.Role == "assistant" {
		eventType = EventAssistantMessage
	}

	eventData := map[string]interface{}{
		"message": message,
		"role":    message.Role,
	}

	if err := m.recordEvent(ctx, sessionID, eventType, eventData); err != nil {
		// 回滚：移除刚添加的消息
		session.Messages = session.Messages[:len(session.Messages)-1]
		session.MessageCount = len(session.Messages)
		return fmt.Errorf("failed to record message event: %w", err)
	}

	// 5. 更新缓存
	m.updateSessionCache(session)

	return nil
}

// GetMessages 获取会话的消息（支持分页）
func (m *Manager) GetMessages(sessionID string, limit int, offset int) ([]llm.Message, error) {
	// 获取会话
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// 应用分页
	total := len(session.Messages)
	start := offset
	end := offset + limit

	if start > total {
		return []llm.Message{}, nil
	}

	if start < 0 {
		start = 0
	}

	if limit <= 0 || end > total {
		end = total
	}

	return session.Messages[start:end], nil
}

// GetLastN 获取最近 N 条消息
func (m *Manager) GetLastN(sessionID string, n int) ([]llm.Message, error) {
	// 获取会话
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	total := len(session.Messages)

	if n <= 0 || n >= total {
		return session.Messages, nil
	}

	return session.Messages[total-n:], nil
}

// GetAllMessages 获取会话的所有消息
func (m *Manager) GetAllMessages(sessionID string) ([]llm.Message, error) {
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return session.Messages, nil
}

// ClearMessages 清空会话的消息历史（保留会话本身）
func (m *Manager) ClearMessages(sessionID string) error {
	// 获取会话
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// 清空消息
	session.Messages = []llm.Message{}
	session.MessageCount = 0

	// 记录事件
	ctx := context.Background()
	if err := m.recordEvent(ctx, sessionID, EventContextCompressed, map[string]interface{}{
		"action": "clear_messages",
		"reason": "manual_clear",
	}); err != nil {
		return fmt.Errorf("failed to record clear event: %w", err)
	}

	// 更新缓存
	m.updateSessionCache(session)

	return nil
}

// GetMessageCount 获取会话的消息数量
func (m *Manager) GetMessageCount(sessionID string) (int, error) {
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return 0, fmt.Errorf("session not found: %s", sessionID)
	}

	return session.MessageCount, nil
}

// ========== 事件操作 ==========

// GetEvents 获取会话的所有事件
func (m *Manager) GetEvents(sessionID string, fromIndex int) ([]*Event, error) {
	ctx := context.Background()
	events, err := m.storage.GetBySessionID(ctx, sessionID, fromIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	return events, nil
}

// GetEventsByType 获取指定类型的事件
func (m *Manager) GetEventsByType(sessionID string, eventType EventType) ([]*Event, error) {
	ctx := context.Background()
	events, err := m.storage.GetByType(ctx, sessionID, eventType)
	if err != nil {
		return nil, fmt.Errorf("failed to get events by type: %w", err)
	}

	return events, nil
}

// GetEventsInRange 获取指定时间范围的事件
func (m *Manager) GetEventsInRange(sessionID string, startTime, endTime time.Time) ([]*Event, error) {
	ctx := context.Background()
	events, err := m.storage.GetRange(ctx, sessionID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get events in range: %w", err)
	}

	return events, nil
}

// ReplaySession 从事件流重建会话状态
func (m *Manager) ReplaySession(sessionID string) (*Session, error) {
	ctx := context.Background()
	session, err := m.storage.Replay(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to replay session: %w", err)
	}

	// 更新到缓存
	m.updateSessionCache(session)

	return session, nil
}

// PruneEvents 清理快照前的事件
func (m *Manager) PruneEvents(sessionID string, beforeIndex int) error {
	ctx := context.Background()
	if err := m.storage.Prune(ctx, sessionID, beforeIndex); err != nil {
		return fmt.Errorf("failed to prune events: %w", err)
	}

	return nil
}

// ========== 快照操作（预留接口，具体实现待 SnapshotStorage 完成）==========

// CreateSnapshot 手动创建快照
func (m *Manager) CreateSnapshot(sessionID string) (*Snapshot, error) {
	if !m.config.EnableSnapshot {
		return nil, fmt.Errorf("snapshot is disabled in config")
	}

	// 获取会话
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// TODO: 实现快照创建逻辑（需要 SnapshotStorage）
	snapshot := &Snapshot{
		SessionID:  sessionID,
		Version:    0, // 应该从 SnapshotStorage 获取下一个版本号
		EventIndex: session.EventCount,
		Session:    session,
		CreatedAt:  time.Now(),
	}

	// 记录快照事件
	ctx := context.Background()
	if err := m.recordEvent(ctx, sessionID, EventSnapshotCreated, map[string]interface{}{
		"version":     snapshot.Version,
		"event_index": snapshot.EventIndex,
		"created_at":  snapshot.CreatedAt.Format(time.RFC3339),
	}); err != nil {
		return nil, fmt.Errorf("failed to record snapshot event: %w", err)
	}

	return snapshot, nil
}

// RestoreFromSnapshot 从快照恢复会话（预留）
func (m *Manager) RestoreFromSnapshot(sessionID string, version int) (*Session, error) {
	if m.snapshotStorage == nil {
		return nil, fmt.Errorf("snapshot storage not configured")
	}

	// TODO: 实现从快照恢复逻辑
	return nil, fmt.Errorf("snapshot restore not implemented yet")
}

// ========== 工具方法 ==========

// GetSessionStats 获取会话统计信息
func (m *Manager) GetSessionStats(sessionID string) (map[string]interface{}, error) {
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	stats := map[string]interface{}{
		"session_id":    session.ID,
		"message_count": session.MessageCount,
		"event_count":   session.EventCount,
		"total_tokens":  session.TotalTokens,
		"created_at":    session.CreatedAt.Format(time.RFC3339),
		"updated_at":    session.UpdatedAt.Format(time.RFC3339),
		"metadata":      session.Metadata,
	}

	// 计算会话年龄
	age := time.Since(session.CreatedAt)
	stats["age_seconds"] = int(age.Seconds())
	stats["age_hours"] = age.Hours()

	// 计算最后活跃时间
	lastActive := time.Since(session.UpdatedAt)
	stats["last_active_seconds"] = int(lastActive.Seconds())

	return stats, nil
}

// ExportSession 导出会话数据（用于备份或调试）
func (m *Manager) ExportSession(sessionID string) (map[string]interface{}, error) {
	// 获取会话
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// 获取所有事件
	ctx := context.Background()
	events, err := m.storage.GetBySessionID(ctx, sessionID, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	export := map[string]interface{}{
		"session": session,
		"events":  events,
	}

	return export, nil
}

// GetCacheStats 获取缓存统计信息
func (m *Manager) GetCacheStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"cached_sessions": len(m.sessions),
	}
}

// GetWithBudget 获取会话消息，自动压缩到预算内
// sessionID: 会话 ID
// maxChars: 最大字符数预算
// summarizer: 摘要生成器（可选，nil 则使用简单截断）
// 返回压缩后的消息列表
func (m *Manager) GetWithBudget(sessionID string, maxChars int, summarizer Summarizer) ([]llm.Message, error) {
	session, err := m.getSessionFromCache(sessionID)
	if err != nil {
		return nil, err
	}

	messages := session.Messages

	// 计算总字符数
	totalChars := 0
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.Type == "text" {
				totalChars += len(block.Text)
			}
		}
	}

	// 未超出预算
	if totalChars <= maxChars {
		return messages, nil
	}

	// 超出预算：调用 Summarizer
	if summarizer != nil {
		ctx := context.Background()
		summary, err := summarizer.Summarize(ctx, messages, maxChars)
		if err == nil {
			return []llm.Message{
				{
					Role: "system",
					Content: []llm.ContentBlock{
						{Type: "text", Text: "【历史摘要】\n" + summary},
					},
				},
			}, nil
		}
	}

	// 无 Summarizer：简单截断（保留最近的消息）
	result := []llm.Message{}
	chars := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgChars := 0
		for _, block := range messages[i].Content {
			if block.Type == "text" {
				msgChars += len(block.Text)
			}
		}

		if chars+msgChars > maxChars {
			break
		}

		result = append([]llm.Message{messages[i]}, result...)
		chars += msgChars
	}

	return result, nil
}
