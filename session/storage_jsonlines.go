package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Ning9527fff/gollm/llm"
)

// JSONLinesStorage JSON Lines 格式事件存储（默认实现）
type JSONLinesStorage struct {
	baseDir       string
	format        string // "json" or "msgpack"
	eventsPerFile int    // 每个文件的事件数（分片大小）
	mu            sync.RWMutex
}

// NewJSONLinesStorage 创建 JSON Lines 存储实例
func NewJSONLinesStorage(options map[string]interface{}) (EventStorage, error) {
	// 解析配置
	baseDir, _ := options["base_dir"].(string)
	if baseDir == "" {
		baseDir = "./data/events"
	}

	format, _ := options["format"].(string)
	if format == "" {
		format = "json"
	}

	eventsPerFile, _ := options["events_per_file"].(int)
	if eventsPerFile == 0 {
		eventsPerFile = 10000
	}

	// 创建基础目录
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &JSONLinesStorage{
		baseDir:       baseDir,
		format:        format,
		eventsPerFile: eventsPerFile,
	}, nil
}

// Append 实现 EventStorage 接口 - 追加单个事件
func (s *JSONLinesStorage) Append(ctx context.Context, event *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 计算事件应该存储到哪个分片文件
	filePath := s.getShardPath(event.SessionID, event.Index)

	// 2. 打开文件（追加模式）
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// 3. 序列化事件（JSON Lines 格式）
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// 4. 写入文件（每行一个 JSON）
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	return nil
}

// getShardPath 计算事件分片文件路径
// 格式: {baseDir}/{sessionID}_{shardID}.log
// 例如: ./data/events/sess_user123_001.log
func (s *JSONLinesStorage) getShardPath(sessionID string, eventIndex int) string {
	shardID := eventIndex / s.eventsPerFile
	filename := fmt.Sprintf("%s_%03d.log", sessionID, shardID)
	return filepath.Join(s.baseDir, filename)
}

// AppendBatch 实现 EventStorage 接口
func (s *JSONLinesStorage) AppendBatch(ctx context.Context, events []*Event) error {
	// TODO: 实现批量追加逻辑
	for _, event := range events {
		if err := s.Append(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// GetBySessionID 实现 EventStorage 接口 - 获取指定会话的事件
func (s *JSONLinesStorage) GetBySessionID(ctx context.Context, sessionID string, fromIndex int) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. 找到所有相关的分片文件
	shardFiles, err := s.findShards(sessionID, fromIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to find shards: %w", err)
	}

	if len(shardFiles) == 0 {
		return []*Event{}, nil // 没有找到文件，返回空列表
	}

	// 2. 读取所有分片文件中的事件
	var events []*Event

	for _, filePath := range shardFiles {
		fileEvents, err := s.readEventsFromFile(filePath, fromIndex)
		if err != nil {
			// 文件可能损坏，记录警告但继续处理其他文件
			fmt.Printf("warning: failed to read events from %s: %v\n", filePath, err)
			continue
		}

		events = append(events, fileEvents...)
	}

	// 3. 按事件索引排序（确保顺序正确）
	sort.Slice(events, func(i, j int) bool {
		return events[i].Index < events[j].Index
	})

	return events, nil
}

// findShards 找到所有可能包含目标事件的分片文件
func (s *JSONLinesStorage) findShards(sessionID string, fromIndex int) ([]string, error) {
	// 计算起始分片
	startShard := fromIndex / s.eventsPerFile

	var shardFiles []string

	// 从起始分片开始查找所有存在的分片文件
	for shardID := startShard; ; shardID++ {
		filePath := s.getShardPathByID(sessionID, shardID)

		// 检查文件是否存在
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			break // 文件不存在，说明已经到达最后一个分片
		} else if err != nil {
			return nil, fmt.Errorf("failed to stat file %s: %w", filePath, err)
		}

		shardFiles = append(shardFiles, filePath)
	}

	return shardFiles, nil
}

// getShardPathByID 根据分片 ID 计算文件路径
func (s *JSONLinesStorage) getShardPathByID(sessionID string, shardID int) string {
	filename := fmt.Sprintf("%s_%03d.log", sessionID, shardID)
	return filepath.Join(s.baseDir, filename)
}

// readEventsFromFile 从单个文件读取事件
func (s *JSONLinesStorage) readEventsFromFile(filePath string, fromIndex int) ([]*Event, error) {
	// 1. 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// 2. 逐行读取并解析 JSON
	var events []*Event
	scanner := bufio.NewScanner(file)

	// 设置更大的缓冲区（默认 64KB，这里设置为 1MB）
	const maxCapacity = 1024 * 1024 // 1 MB
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行
		if line == "" {
			continue
		}

		// 解析 JSON
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// JSON 解析失败，记录警告但继续处理
			fmt.Printf("warning: failed to parse line %d in %s: %v\n", lineNum, filePath, err)
			continue
		}

		// 过滤：只返回 Index >= fromIndex 的事件
		if event.Index >= fromIndex {
			events = append(events, &event)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	return events, nil
}

// GetByType 实现 EventStorage 接口
func (s *JSONLinesStorage) GetByType(ctx context.Context, sessionID string, eventType EventType) ([]*Event, error) {
	// TODO: 实现按类型查询
	events, err := s.GetBySessionID(ctx, sessionID, 0)
	if err != nil {
		return nil, err
	}

	var filtered []*Event
	for _, event := range events {
		if event.Type == eventType {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

// GetRange 实现 EventStorage 接口
func (s *JSONLinesStorage) GetRange(ctx context.Context, sessionID string, startTime, endTime time.Time) ([]*Event, error) {
	// TODO: 实现按时间范围查询
	events, err := s.GetBySessionID(ctx, sessionID, 0)
	if err != nil {
		return nil, err
	}

	var filtered []*Event
	for _, event := range events {
		if event.Timestamp.After(startTime) && event.Timestamp.Before(endTime) {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

// Replay 实现 EventStorage 接口 - 从事件流重建会话状态
func (s *JSONLinesStorage) Replay(ctx context.Context, sessionID string) (*Session, error) {
	// 1. 获取所有事件（从索引 0 开始）
	events, err := s.GetBySessionID(ctx, sessionID, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	if len(events) == 0 {
		return nil, fmt.Errorf("no events found for session: %s", sessionID)
	}

	// 2. 初始化会话对象
	session := &Session{
		ID:           sessionID,
		Messages:     []llm.Message{},
		Metadata:     make(map[string]string),
		EventCount:   0,
		TotalTokens:  0,
		MessageCount: 0,
	}

	// 3. 遍历事件，重建状态
	for _, event := range events {
		if err := s.applyEvent(session, event); err != nil {
			// 记录警告但继续处理
			fmt.Printf("warning: failed to apply event %s: %v\n", event.ID, err)
		}
		session.EventCount++
	}

	// 4. 更新消息计数
	session.MessageCount = len(session.Messages)

	return session, nil
}

// applyEvent 将单个事件应用到会话状态
func (s *JSONLinesStorage) applyEvent(session *Session, event *Event) error {
	switch event.Type {
	case EventSessionCreated:
		// 会话创建事件
		if metadata, ok := event.Data["metadata"].(map[string]interface{}); ok {
			for key, value := range metadata {
				if strValue, ok := value.(string); ok {
					session.Metadata[key] = strValue
				}
			}
		}

		// 设置创建时间
		if createdAt, ok := event.Data["created_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
				session.CreatedAt = t
			}
		} else {
			session.CreatedAt = event.Timestamp
		}

		session.UpdatedAt = event.Timestamp

	case EventUserMessage, EventAssistantMessage:
		// 用户消息或助手消息
		if msgData, ok := event.Data["message"]; ok {
			// 将 map 转换为 JSON 再反序列化为 Message
			msgJSON, err := json.Marshal(msgData)
			if err != nil {
				return fmt.Errorf("failed to marshal message: %w", err)
			}

			var message llm.Message
			if err := json.Unmarshal(msgJSON, &message); err != nil {
				return fmt.Errorf("failed to unmarshal message: %w", err)
			}

			session.Messages = append(session.Messages, message)
		}

		session.UpdatedAt = event.Timestamp

	case EventToolCall:
		// 工具调用事件（可能包含在消息中）
		// TODO: 根据实际需求处理
		session.UpdatedAt = event.Timestamp

	case EventToolResult:
		// 工具结果事件
		// TODO: 根据实际需求处理
		session.UpdatedAt = event.Timestamp

	case EventContextCompressed:
		// 上下文压缩事件
		if action, ok := event.Data["action"].(string); ok && action == "clear_messages" {
			// 清空消息历史
			session.Messages = []llm.Message{}
		}
		session.UpdatedAt = event.Timestamp

	case EventSnapshotCreated:
		// 快照创建事件（不影响会话状态）
		session.UpdatedAt = event.Timestamp

	case EventSessionClosed:
		// 会话关闭事件
		session.UpdatedAt = event.Timestamp

	default:
		// 未知事件类型，记录警告
		fmt.Printf("warning: unknown event type: %s\n", event.Type)
	}

	return nil
}

// Prune 实现 EventStorage 接口 - 清理指定索引之前的事件
func (s *JSONLinesStorage) Prune(ctx context.Context, sessionID string, beforeIndex int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if beforeIndex <= 0 {
		return nil // 没有需要清理的事件
	}

	// 1. 计算需要删除的分片
	maxShardToDelete := (beforeIndex - 1) / s.eventsPerFile

	// 2. 删除完整的分片文件
	deletedCount := 0
	for shardID := 0; shardID <= maxShardToDelete; shardID++ {
		filePath := s.getShardPathByID(sessionID, shardID)

		// 检查文件是否存在
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			continue // 文件不存在，跳过
		}

		// 删除文件
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to remove shard %s: %w", filePath, err)
		}

		deletedCount++
	}

	// 3. 处理边界分片（可能只需要删除部分事件）
	boundaryShardID := beforeIndex / s.eventsPerFile
	boundaryFilePath := s.getShardPathByID(sessionID, boundaryShardID)

	// 检查边界分片是否存在
	if _, err := os.Stat(boundaryFilePath); err == nil {
		// 读取边界分片，只保留 Index >= beforeIndex 的事件
		if err := s.prunePartialShard(boundaryFilePath, beforeIndex); err != nil {
			return fmt.Errorf("failed to prune boundary shard: %w", err)
		}
	}

	return nil
}

// prunePartialShard 清理分片文件中的部分事件
func (s *JSONLinesStorage) prunePartialShard(filePath string, beforeIndex int) error {
	// 1. 读取所有事件
	events, err := s.readEventsFromFile(filePath, 0)
	if err != nil {
		return fmt.Errorf("failed to read events: %w", err)
	}

	// 2. 过滤出需要保留的事件
	var keptEvents []*Event
	for _, event := range events {
		if event.Index >= beforeIndex {
			keptEvents = append(keptEvents, event)
		}
	}

	// 3. 如果所有事件都被删除，删除文件
	if len(keptEvents) == 0 {
		return os.Remove(filePath)
	}

	// 4. 重写文件（只包含保留的事件）
	tempPath := filePath + ".tmp"
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	// 写入保留的事件
	for _, event := range keptEvents {
		data, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("failed to marshal event: %w", err)
		}

		if _, err := tempFile.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("failed to write event: %w", err)
		}
	}

	// 5. 原子替换原文件
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// Delete 实现 EventStorage 接口 - 删除会话的所有事件
func (s *JSONLinesStorage) Delete(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 查找所有分片文件（使用通配符）
	pattern := filepath.Join(s.baseDir, sessionID+"_*.log")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob files: %w", err)
	}

	if len(files) == 0 {
		return nil // 没有找到文件，直接返回成功
	}

	// 2. 删除所有分片文件
	deletedCount := 0
	var lastErr error

	for _, filePath := range files {
		if err := os.Remove(filePath); err != nil {
			// 记录错误但继续删除其他文件
			lastErr = err
			fmt.Printf("warning: failed to delete %s: %v\n", filePath, err)
		} else {
			deletedCount++
		}
	}

	// 3. 如果有文件删除失败，返回错误
	if lastErr != nil && deletedCount == 0 {
		return fmt.Errorf("failed to delete any files: %w", lastErr)
	}

	return nil
}

// Close 实现 EventStorage 接口
func (s *JSONLinesStorage) Close() error {
	// TODO: 关闭文件句柄等资源
	return nil
}
