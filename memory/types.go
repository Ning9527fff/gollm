package memory

import (
	"time"

	"github.com/Ning9527fff/gollm/session"
)

// CacheEntry 缓存条目（包含 TTL 信息）
type CacheEntry struct {
	Session     *session.Session // 会话对象
	ExpiresAt   time.Time        // 过期时间
	AccessCount int              // 访问次数
	CreatedAt   time.Time        // 创建时间
}

// isExpired 检查条目是否过期
func (e *CacheEntry) isExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// Config 记忆管理配置
type Config struct {
	MaxSize           int           // 最大缓存条目数
	TTL               time.Duration // 条目过期时间（0 表示永不过期）
	CleanupInterval   time.Duration // 清理过期条目的间隔（0 表示禁用自动清理）
	EnableStatistics  bool          // 是否启用统计信息
}

// DefaultConfig 默认配置
var DefaultConfig = Config{
	MaxSize:          1000,              // 最多缓存 1000 个会话
	TTL:              1 * time.Hour,     // 1 小时过期
	CleanupInterval:  5 * time.Minute,   // 每 5 分钟清理一次
	EnableStatistics: true,              // 启用统计
}

// Stats 缓存统计信息
type Stats struct {
	Hits       int64   // 缓存命中次数
	Misses     int64   // 缓存未命中次数
	Evictions  int64   // 淘汰次数
	Expirations int64  // 过期次数
	Size       int     // 当前缓存大小
	HitRate    float64 // 命中率
}
