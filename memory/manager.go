package memory

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/Ning9527fff/gollm/session"
)

// Manager 记忆管理器（LRU + TTL）
type Manager struct {
	config Config
	cache  *lru.Cache[string, *CacheEntry]
	mu     sync.RWMutex

	// 统计信息（使用 atomic 保证并发安全）
	hits        atomic.Int64
	misses      atomic.Int64
	evictions   atomic.Int64
	expirations atomic.Int64

	// 清理协程控制
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

// NewManager 创建记忆管理器
func NewManager(config Config) (*Manager, error) {
	// 参数验证
	if config.MaxSize <= 0 {
		return nil, fmt.Errorf("MaxSize must be greater than 0")
	}

	// 创建 LRU 缓存
	cache, err := lru.NewWithEvict(config.MaxSize, func(key string, value *CacheEntry) {
		// 淘汰回调
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create LRU cache: %w", err)
	}

	mgr := &Manager{
		config:      config,
		cache:       cache,
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	// 启动自动清理协程
	if config.CleanupInterval > 0 {
		go mgr.cleanupLoop()
	}

	return mgr, nil
}

// NewDefaultManager 使用默认配置创建管理器
func NewDefaultManager() (*Manager, error) {
	return NewManager(DefaultConfig)
}

// Close 关闭管理器
func (m *Manager) Close() error {
	// 停止清理协程
	if m.config.CleanupInterval > 0 {
		close(m.stopCleanup)
		<-m.cleanupDone // 等待清理协程退出
	}

	// 清空缓存
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache.Purge()

	return nil
}

// ========== 缓存操作 ==========

// Set 设置缓存（如果 ttl 为 0，使用配置的默认 TTL）
func (m *Manager) Set(sessionID string, sess *session.Session, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 使用传入的 TTL，如果为 0 则使用配置的默认值
	if ttl == 0 {
		ttl = m.config.TTL
	}

	// 创建缓存条目
	entry := &CacheEntry{
		Session:     sess,
		ExpiresAt:   time.Now().Add(ttl),
		AccessCount: 0,
		CreatedAt:   time.Now(),
	}

	// 如果 TTL 为 0，表示永不过期
	if ttl == 0 {
		entry.ExpiresAt = time.Time{} // 零值表示永不过期
	}

	// 存入 LRU 缓存（如果超过容量，会自动淘汰最少使用的条目）
	evicted := m.cache.Add(sessionID, entry)

	// 如果发生了淘汰
	if evicted {
		m.evictions.Add(1)
	}

	return nil
}

// Get 获取缓存（自动检查过期）
func (m *Manager) Get(sessionID string) (*session.Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 从 LRU 缓存获取
	entry, found := m.cache.Get(sessionID)

	if !found {
		// 缓存未命中
		m.misses.Add(1)
		return nil, false
	}

	// 检查是否过期
	if !entry.ExpiresAt.IsZero() && entry.isExpired() {
		// 过期，删除并返回未命中
		m.cache.Remove(sessionID)
		m.expirations.Add(1)
		m.misses.Add(1)
		return nil, false
	}

	// 缓存命中
	entry.AccessCount++
	m.hits.Add(1)

	return entry.Session, true
}

// Delete 删除缓存
func (m *Manager) Delete(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache.Remove(sessionID)
	return nil
}

// Clear 清空所有缓存
func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache.Purge()

	// 重置统计信息
	m.hits.Store(0)
	m.misses.Store(0)
	m.evictions.Store(0)
	m.expirations.Store(0)

	return nil
}

// Contains 检查缓存中是否存在（不影响 LRU 顺序）
func (m *Manager) Contains(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.cache.Contains(sessionID)
}

// Len 返回当前缓存大小
func (m *Manager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.cache.Len()
}

// ========== TTL 过期机制 ==========

// cleanupLoop 定期清理过期条目的后台协程
func (m *Manager) cleanupLoop() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()
	defer close(m.cleanupDone)

	for {
		select {
		case <-ticker.C:
			// 执行清理
			m.removeExpired()

		case <-m.stopCleanup:
			// 收到停止信号，退出协程
			return
		}
	}
}

// removeExpired 遍历并删除所有过期的缓存条目
func (m *Manager) removeExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取所有 key
	keys := m.cache.Keys()
	removedCount := 0

	for _, key := range keys {
		// 查看条目（不影响 LRU 顺序）
		entry, found := m.cache.Peek(key)
		if !found {
			continue
		}

		// 检查是否过期
		if !entry.ExpiresAt.IsZero() && entry.isExpired() {
			m.cache.Remove(key)
			m.expirations.Add(1)
			removedCount++
		}
	}

	return removedCount
}

// RemoveExpiredNow 手动触发清理过期条目（同步方法）
func (m *Manager) RemoveExpiredNow() int {
	return m.removeExpired()
}

// ========== 统计信息 ==========

// Stats 获取缓存统计信息
func (m *Manager) Stats() Stats {
	m.mu.RLock()
	size := m.cache.Len()
	m.mu.RUnlock()

	// 读取原子计数器
	hits := m.hits.Load()
	misses := m.misses.Load()
	evictions := m.evictions.Load()
	expirations := m.expirations.Load()

	// 计算命中率
	total := hits + misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return Stats{
		Hits:        hits,
		Misses:      misses,
		Evictions:   evictions,
		Expirations: expirations,
		Size:        size,
		HitRate:     hitRate,
	}
}

// ResetStats 重置统计信息
func (m *Manager) ResetStats() {
	m.hits.Store(0)
	m.misses.Store(0)
	m.evictions.Store(0)
	m.expirations.Store(0)
}

// GetAllEntries 获取所有缓存条目的详细信息（用于调试）
func (m *Manager) GetAllEntries() map[string]*CacheEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*CacheEntry)
	keys := m.cache.Keys()

	for _, key := range keys {
		if entry, found := m.cache.Peek(key); found {
			result[key] = entry
		}
	}

	return result
}
