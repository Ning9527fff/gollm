package context

import (
	"context"
	"fmt"

	"github.com/Ning9527fff/gollm/llm"
)

// ========== 滑动窗口策略 ==========

// SlidingWindowStrategy 滑动窗口压缩策略
// 保留最近 N 条消息，丢弃更早的消息
type SlidingWindowStrategy struct {
	WindowSize int // 窗口大小（保留的消息数量）
}

// NewSlidingWindowStrategy 创建滑动窗口策略
func NewSlidingWindowStrategy(windowSize int) *SlidingWindowStrategy {
	if windowSize <= 0 {
		windowSize = 10 // 默认保留 10 条
	}

	return &SlidingWindowStrategy{
		WindowSize: windowSize,
	}
}

// Compress 实现 CompressionStrategy 接口
func (s *SlidingWindowStrategy) Compress(ctx context.Context, messages []llm.Message, maxTokens int) ([]llm.Message, error) {
	if len(messages) <= s.WindowSize {
		// 消息数量未超过窗口大小，直接返回
		return messages, nil
	}

	// 保留最近 WindowSize 条消息
	startIndex := len(messages) - s.WindowSize
	return messages[startIndex:], nil
}

// Name 实现 CompressionStrategy 接口
func (s *SlidingWindowStrategy) Name() string {
	return fmt.Sprintf("SlidingWindow(size=%d)", s.WindowSize)
}

// ========== 无压缩策略 ==========

// NoCompressionStrategy 不进行任何压缩
type NoCompressionStrategy struct{}

// NewNoCompressionStrategy 创建无压缩策略
func NewNoCompressionStrategy() *NoCompressionStrategy {
	return &NoCompressionStrategy{}
}

// Compress 实现 CompressionStrategy 接口
func (s *NoCompressionStrategy) Compress(ctx context.Context, messages []llm.Message, maxTokens int) ([]llm.Message, error) {
	return messages, nil
}

// Name 实现 CompressionStrategy 接口
func (s *NoCompressionStrategy) Name() string {
	return "NoCompression"
}

// ========== 简单截断策略 ==========

// TruncateStrategy 简单截断策略
// 当消息数量超过限制时，从头部开始删除
type TruncateStrategy struct {
	MaxMessages int // 最大消息数量
}

// NewTruncateStrategy 创建截断策略
func NewTruncateStrategy(maxMessages int) *TruncateStrategy {
	if maxMessages <= 0 {
		maxMessages = 50 // 默认最多 50 条
	}

	return &TruncateStrategy{
		MaxMessages: maxMessages,
	}
}

// Compress 实现 CompressionStrategy 接口
func (s *TruncateStrategy) Compress(ctx context.Context, messages []llm.Message, maxTokens int) ([]llm.Message, error) {
	if len(messages) <= s.MaxMessages {
		return messages, nil
	}

	// 删除头部消息
	startIndex := len(messages) - s.MaxMessages
	return messages[startIndex:], nil
}

// Name 实现 CompressionStrategy 接口
func (s *TruncateStrategy) Name() string {
	return fmt.Sprintf("Truncate(max=%d)", s.MaxMessages)
}
