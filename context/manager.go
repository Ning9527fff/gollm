package context

import (
	"context"
	"fmt"

	"github.com/Ning9527fff/gollm/llm"
	"github.com/Ning9527fff/gollm/memory"
	"github.com/Ning9527fff/gollm/session"
)

// Manager 上下文管理器
type Manager struct {
	config         Config
	sessionManager *session.Manager
	memoryManager  *memory.Manager
}

// NewManager 创建上下文管理器
func NewManager(config Config, sessionMgr *session.Manager, memoryMgr *memory.Manager) (*Manager, error) {
	// 参数验证
	if sessionMgr == nil {
		return nil, fmt.Errorf("sessionManager cannot be nil")
	}

	// 如果未设置默认策略，使用滑动窗口
	if config.DefaultStrategy == nil {
		config.DefaultStrategy = NewSlidingWindowStrategy(10)
	}

	return &Manager{
		config:         config,
		sessionManager: sessionMgr,
		memoryManager:  memoryMgr,
	}, nil
}

// NewDefaultManager 使用默认配置创建管理器
func NewDefaultManager(sessionMgr *session.Manager, memoryMgr *memory.Manager) (*Manager, error) {
	config := Config{
		DefaultStrategy:  NewSlidingWindowStrategy(10),
		EnableTokenCount: false, // 暂未实现 Token 计数
	}

	return NewManager(config, sessionMgr, memoryMgr)
}

// ========== 核心功能 ==========

// BuildContext 构建上下文（核心方法）
func (m *Manager) BuildContext(sessionID string, options ContextOptions) ([]llm.Message, error) {
	ctx := context.Background()

	// 1. 从缓存或 SessionManager 获取会话
	sess, err := m.getSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// 2. 获取消息历史
	messages := sess.Messages

	// 如果没有消息，返回空列表
	if len(messages) == 0 {
		return m.buildWithSystemPrompt([]llm.Message{}, options), nil
	}

	// 3. 应用消息数量限制（如果设置）
	if options.MaxMessages > 0 && len(messages) > options.MaxMessages {
		messages = messages[len(messages)-options.MaxMessages:]
	}

	// 4. 应用压缩策略
	strategy := options.Strategy
	if strategy == nil {
		strategy = m.config.DefaultStrategy
	}

	compressedMessages, err := strategy.Compress(ctx, messages, options.MaxTokens)
	if err != nil {
		return nil, fmt.Errorf("compression failed: %w", err)
	}

	// 5. 添加 system prompt（如果需要）
	result := m.buildWithSystemPrompt(compressedMessages, options)

	return result, nil
}

// getSession 从缓存或 SessionManager 获取会话
func (m *Manager) getSession(sessionID string) (*session.Session, error) {
	// 1. 尝试从缓存获取
	if m.memoryManager != nil {
		if sess, found := m.memoryManager.Get(sessionID); found {
			return sess, nil
		}
	}

	// 2. 从 SessionManager 获取
	sess, err := m.sessionManager.Get(sessionID)
	if err != nil {
		return nil, err
	}

	// 3. 更新缓存
	if m.memoryManager != nil {
		m.memoryManager.Set(sessionID, sess, 0) // 使用默认 TTL
	}

	return sess, nil
}

// buildWithSystemPrompt 添加 system prompt（如果需要）
func (m *Manager) buildWithSystemPrompt(messages []llm.Message, options ContextOptions) []llm.Message {
	if !options.IncludeSystem || options.SystemPrompt == "" {
		return messages
	}

	// 创建 system message
	systemMsg := llm.Message{
		Role: "system",
		Content: []llm.ContentBlock{
			{
				Type: "text",
				Text: options.SystemPrompt,
			},
		},
	}

	// 将 system message 放在最前面
	result := make([]llm.Message, 0, len(messages)+1)
	result = append(result, systemMsg)
	result = append(result, messages...)

	return result
}

// ========== 工具方法 ==========

// Compress 直接压缩消息列表（不依赖会话）
func (m *Manager) Compress(messages []llm.Message, strategy CompressionStrategy, maxTokens int) ([]llm.Message, error) {
	ctx := context.Background()

	if strategy == nil {
		strategy = m.config.DefaultStrategy
	}

	return strategy.Compress(ctx, messages, maxTokens)
}

// EstimateTokens 估算消息列表的 Token 数量
// 注意：这是粗略估算，实际 Token 数量取决于具体的 tokenizer
func (m *Manager) EstimateTokens(messages []llm.Message) int {
	totalChars := 0

	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.Type == "text" {
				totalChars += len(block.Text)
			}
		}
	}

	// 粗略估算：平均 4 个字符 = 1 token
	// 这对英文较准确，中文可能需要调整（中文约 1.5-2 字符 = 1 token）
	return totalChars / 4
}

// TruncateToLimit 截断消息列表以符合 Token 限制
func (m *Manager) TruncateToLimit(messages []llm.Message, maxTokens int) []llm.Message {
	if maxTokens <= 0 {
		return messages
	}

	// 从后往前累加，直到超过限制
	estimatedTokens := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := m.estimateMessageTokens(messages[i])

		if estimatedTokens+msgTokens > maxTokens {
			// 超过限制，返回从 i+1 开始的消息
			if i+1 < len(messages) {
				return messages[i+1:]
			}
			return []llm.Message{}
		}

		estimatedTokens += msgTokens
	}

	// 未超过限制，返回全部
	return messages
}

// estimateMessageTokens 估算单条消息的 Token 数
func (m *Manager) estimateMessageTokens(message llm.Message) int {
	totalChars := 0

	for _, block := range message.Content {
		if block.Type == "text" {
			totalChars += len(block.Text)
		}
	}

	return totalChars / 4
}

// GetLastNMessages 从会话获取最近 N 条消息
func (m *Manager) GetLastNMessages(sessionID string, n int) ([]llm.Message, error) {
	sess, err := m.getSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	total := len(sess.Messages)
	if n <= 0 || n >= total {
		return sess.Messages, nil
	}

	return sess.Messages[total-n:], nil
}

// GetMessageCount 获取会话的消息数量
func (m *Manager) GetMessageCount(sessionID string) (int, error) {
	sess, err := m.getSession(sessionID)
	if err != nil {
		return 0, fmt.Errorf("failed to get session: %w", err)
	}

	return len(sess.Messages), nil
}

// SetDefaultStrategy 设置默认压缩策略
func (m *Manager) SetDefaultStrategy(strategy CompressionStrategy) {
	m.config.DefaultStrategy = strategy
}

// GetDefaultStrategy 获取默认压缩策略
func (m *Manager) GetDefaultStrategy() CompressionStrategy {
	return m.config.DefaultStrategy
}
