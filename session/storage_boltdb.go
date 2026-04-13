package session

import (
	"context"
	"fmt"
	"time"
)

// BoltDBStorage BoltDB 事件存储（预留，后续实现）
type BoltDBStorage struct {
	dbPath string
}

// NewBoltDBStorage 创建 BoltDB 存储实例
func NewBoltDBStorage(options map[string]interface{}) (EventStorage, error) {
	dbPath, _ := options["db_path"].(string)
	if dbPath == "" {
		dbPath = "./data/events.db"
	}

	// TODO: 打开 BoltDB 连接

	return &BoltDBStorage{
		dbPath: dbPath,
	}, nil
}

// Append 实现 EventStorage 接口
func (s *BoltDBStorage) Append(ctx context.Context, event *Event) error {
	return fmt.Errorf("BoltDBStorage not implemented yet")
}

// AppendBatch 实现 EventStorage 接口
func (s *BoltDBStorage) AppendBatch(ctx context.Context, events []*Event) error {
	return fmt.Errorf("BoltDBStorage not implemented yet")
}

// GetBySessionID 实现 EventStorage 接口
func (s *BoltDBStorage) GetBySessionID(ctx context.Context, sessionID string, fromIndex int) ([]*Event, error) {
	return nil, fmt.Errorf("BoltDBStorage not implemented yet")
}

// GetByType 实现 EventStorage 接口
func (s *BoltDBStorage) GetByType(ctx context.Context, sessionID string, eventType EventType) ([]*Event, error) {
	return nil, fmt.Errorf("BoltDBStorage not implemented yet")
}

// GetRange 实现 EventStorage 接口
func (s *BoltDBStorage) GetRange(ctx context.Context, sessionID string, startTime, endTime time.Time) ([]*Event, error) {
	return nil, fmt.Errorf("BoltDBStorage not implemented yet")
}

// Replay 实现 EventStorage 接口
func (s *BoltDBStorage) Replay(ctx context.Context, sessionID string) (*Session, error) {
	return nil, fmt.Errorf("BoltDBStorage not implemented yet")
}

// Prune 实现 EventStorage 接口
func (s *BoltDBStorage) Prune(ctx context.Context, sessionID string, beforeIndex int) error {
	return fmt.Errorf("BoltDBStorage not implemented yet")
}

// Delete 实现 EventStorage 接口
func (s *BoltDBStorage) Delete(ctx context.Context, sessionID string) error {
	return fmt.Errorf("BoltDBStorage not implemented yet")
}

// Close 实现 EventStorage 接口
func (s *BoltDBStorage) Close() error {
	return nil
}
