package session

import (
	"context"
	"fmt"
	"time"
)

// EventStorage 事件存储接口（抽象层）
// 所有具体的存储实现（JSONLines, BoltDB, PostgreSQL 等）都必须实现此接口
type EventStorage interface {
	// Append 追加单个事件
	Append(ctx context.Context, event *Event) error

	// AppendBatch 批量追加事件（性能优化）
	AppendBatch(ctx context.Context, events []*Event) error

	// GetBySessionID 获取指定会话的事件
	// fromIndex: 从第几个事件开始获取（0 表示从头开始）
	GetBySessionID(ctx context.Context, sessionID string, fromIndex int) ([]*Event, error)

	// GetByType 获取指定会话的特定类型事件
	GetByType(ctx context.Context, sessionID string, eventType EventType) ([]*Event, error)

	// GetRange 获取指定时间范围内的事件
	GetRange(ctx context.Context, sessionID string, startTime, endTime time.Time) ([]*Event, error)

	// Replay 从事件流重建会话状态
	Replay(ctx context.Context, sessionID string) (*Session, error)

	// Prune 清理指定索引之前的事件（通常在快照后调用）
	// beforeIndex: 删除索引小于此值的所有事件
	Prune(ctx context.Context, sessionID string, beforeIndex int) error

	// Delete 删除指定会话的所有事件
	Delete(ctx context.Context, sessionID string) error

	// Close 关闭存储连接，释放资源
	Close() error
}

// NewEventStorage 工厂方法，根据配置创建存储实例
func NewEventStorage(config EventStorageConfig) (EventStorage, error) {
	switch config.Type {
	case "jsonlines", "": // 默认使用 JSON Lines
		return NewJSONLinesStorage(config.Options)
	case "boltdb":
		return NewBoltDBStorage(config.Options)
	case "postgres":
		return nil, fmt.Errorf("postgres storage not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.Type)
	}
}

// Snapshot 快照结构（预留，后续实现 SnapshotStore）
type Snapshot struct {
	SessionID      string    `json:"session_id" msgpack:"session_id"`                               // 会话 ID
	Version        int       `json:"version" msgpack:"version"`                                     // 快照版本号
	EventIndex     int       `json:"event_index" msgpack:"event_index"`                             // 快照对应的事件索引
	Session        *Session  `json:"session" msgpack:"session"`                                     // 会话完整状态
	CreatedAt      time.Time `json:"created_at" msgpack:"created_at"`                               // 快照创建时间
	CompressedData []byte    `json:"compressed_data,omitempty" msgpack:"compressed_data,omitempty"` // 压缩后的数据（可选）
}

// SnapshotMetadata 快照元数据
type SnapshotMetadata struct {
	SessionID  string    `json:"session_id"`
	Version    int       `json:"version"`
	EventIndex int       `json:"event_index"`
	CreatedAt  time.Time `json:"created_at"`
	Size       int64     `json:"size"` // 字节数
}

// SnapshotStorage 快照存储接口（预留，后续实现）
type SnapshotStorage interface {
	// Save 保存快照
	Save(ctx context.Context, snapshot *Snapshot) error

	// GetLatest 获取最新快照
	GetLatest(ctx context.Context, sessionID string) (*Snapshot, error)

	// Get 获取指定版本的快照
	Get(ctx context.Context, sessionID string, version int) (*Snapshot, error)

	// List 列出指定会话的所有快照元数据
	List(ctx context.Context, sessionID string) ([]*SnapshotMetadata, error)

	// Delete 删除指定版本的快照
	Delete(ctx context.Context, sessionID string, version int) error

	// Prune 保留最近 N 个快照，删除其他的
	Prune(ctx context.Context, sessionID string, keepLast int) error

	// Close 关闭存储连接
	Close() error
}
