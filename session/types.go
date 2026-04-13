package session

import (
	"time"

	"github.com/Ning9527fff/gollm/llm"
)

// EventType 事件类型
type EventType string

const (
	EventSessionCreated    EventType = "session_created"    // 会话创建
	EventUserMessage       EventType = "user_message"       // 用户发送消息
	EventAssistantMessage  EventType = "assistant_message"  // 助手回复消息
	EventToolCall          EventType = "tool_call"          // 工具调用
	EventToolResult        EventType = "tool_result"        // 工具执行结果
	EventContextCompressed EventType = "context_compressed" // 上下文被压缩
	EventSnapshotCreated   EventType = "snapshot_created"   // 快照创建
	EventSessionClosed     EventType = "session_closed"     // 会话关闭
)

// Event 状态事件（Event Sourcing）
type Event struct {
	ID        string                 `json:"id" msgpack:"id"`                 // 事件 ID
	SessionID string                 `json:"session_id" msgpack:"session_id"` // 所属会话 ID
	Type      EventType              `json:"type" msgpack:"type"`             // 事件类型
	Timestamp time.Time              `json:"timestamp" msgpack:"timestamp"`   // 事件时间戳
	Index     int                    `json:"index" msgpack:"index"`           // 事件索引（用于排序）
	Data      map[string]interface{} `json:"data" msgpack:"data"`             // 事件数据
	Metadata  map[string]string      `json:"metadata,omitempty" msgpack:"metadata,omitempty"` // 元数据
}

// Session 会话状态
type Session struct {
	ID         string            `json:"id" msgpack:"id"`                   // 会话 ID
	Messages   []llm.Message     `json:"messages" msgpack:"messages"`       // 消息历史
	Metadata   map[string]string `json:"metadata" msgpack:"metadata"`       // 会话元数据
	CreatedAt  time.Time         `json:"created_at" msgpack:"created_at"`   // 创建时间
	UpdatedAt  time.Time         `json:"updated_at" msgpack:"updated_at"`   // 最后更新时间
	EventCount int               `json:"event_count" msgpack:"event_count"` // 事件总数

	// 统计信息
	TotalTokens  int `json:"total_tokens,omitempty" msgpack:"total_tokens,omitempty"`   // 累计 Token 消耗
	MessageCount int `json:"message_count,omitempty" msgpack:"message_count,omitempty"` // 消息总数
}

// SessionFilter 会话查询过滤器
type SessionFilter struct {
	Metadata      map[string]string // 按元数据过滤
	CreatedAfter  time.Time         // 创建时间筛选
	CreatedBefore time.Time
	Limit         int // 分页限制
	Offset        int // 分页偏移
}

// Config 会话管理配置
type Config struct {
	Storage           EventStorageConfig // 存储配置
	EnableEventLog    bool               // 是否启用事件日志
	EnableSnapshot    bool               // 是否启用快照
	SnapshotInterval  int                // 快照间隔（每 N 个事件创建一次快照）
	MaxMessageHistory int                // 最大消息历史数量（0 表示无限制）
	AutoCompress      bool               // 是否自动压缩历史
	TTL               time.Duration      // 会话 TTL（0 表示永不过期）
}

// EventStorageConfig 事件存储配置
type EventStorageConfig struct {
	Type    string                 // 存储类型: "jsonlines", "boltdb", "postgres"
	Options map[string]interface{} // 类型特定选项
}

// DefaultConfig 默认配置
var DefaultConfig = Config{
	Storage: EventStorageConfig{
		Type: "jsonlines", // 默认使用 JSON Lines
		Options: map[string]interface{}{
			"base_dir":        "./data/events",
			"format":          "json",
			"events_per_file": 10000,
		},
	},
	EnableEventLog:    true,
	EnableSnapshot:    true,
	SnapshotInterval:  10,             // 每 10 个事件创建快照
	MaxMessageHistory: 100,            // 最多保留 100 条消息
	AutoCompress:      false,          // 默认不自动压缩
	TTL:               24 * time.Hour, // 默认 24 小时过期
}
