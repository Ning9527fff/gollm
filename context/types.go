package context

import (
	"context"

	"github.com/Ning9527fff/gollm/llm"
)

// CompressionStrategy 上下文压缩策略接口
type CompressionStrategy interface {
	// Compress 压缩消息列表
	// messages: 原始消息列表
	// maxTokens: 最大 Token 限制
	// 返回: 压缩后的消息列表和错误
	Compress(ctx context.Context, messages []llm.Message, maxTokens int) ([]llm.Message, error)

	// Name 返回策略名称
	Name() string
}

// ContextOptions 上下文构建选项
type ContextOptions struct {
	MaxTokens       int                  // 最大 Token 限制（0 表示不限制）
	MaxMessages     int                  // 最大消息数量（0 表示不限制）
	Strategy        CompressionStrategy  // 压缩策略（nil 表示使用默认策略）
	IncludeSystem   bool                 // 是否包含 system prompt
	SystemPrompt    string               // system prompt 内容
	RetrievalQuery  string               // RAG 检索查询（预留）
	RetrievalTopK   int                  // RAG 检索数量（预留）
}

// DefaultContextOptions 默认上下文选项
var DefaultContextOptions = ContextOptions{
	MaxTokens:     4000,  // 默认 4000 tokens
	MaxMessages:   0,     // 不限制消息数量
	IncludeSystem: false, // 默认不包含 system prompt
	RetrievalTopK: 5,     // 默认检索 5 个文档
}

// Config 上下文管理器配置
type Config struct {
	DefaultStrategy CompressionStrategy // 默认压缩策略
	EnableTokenCount bool                // 是否启用 Token 计数（预留）
}
