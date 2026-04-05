package llm

import "context"

// LLM 统一的 LLM 接口
type LLM interface {
	// Generate 生成响应（阻塞调用）
	Generate(ctx context.Context, messages []Message, opts ...Option) (*Response, error)

	// Stream 流式生成响应
	Stream(ctx context.Context, messages []Message, opts ...Option) (<-chan Chunk, error)
}

// Option 配置选项函数
type Option func(*Options)

// Options 请求选项
type Options struct {
	Model       string       // 模型名称（如 "gpt-4o", "claude-sonnet-4-6"）
	Temperature float32      // 温度参数 [0, 2]
	MaxTokens   int          // 最大输出 token 数
	Tools       []Tool       // 可用的工具列表
	Cache       *CacheConfig // 缓存配置
}

// WithModel 设置模型
func WithModel(model string) Option {
	return func(o *Options) {
		o.Model = model
	}
}

// WithTemperature 设置温度参数
func WithTemperature(temp float32) Option {
	return func(o *Options) {
		o.Temperature = temp
	}
}

// WithMaxTokens 设置最大输出 token 数
func WithMaxTokens(max int) Option {
	return func(o *Options) {
		o.MaxTokens = max
	}
}

// WithTools 设置可用工具
func WithTools(tools []Tool) Option {
	return func(o *Options) {
		o.Tools = tools
	}
}

// WithCache 设置缓存配置
func WithCache(config CacheConfig) Option {
	return func(o *Options) {
		o.Cache = &config
	}
}
