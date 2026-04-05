package anthropic

import (
	"context"
	"fmt"

	"github.com/Ning9527fff/gollm/llm"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Anthropic 实现 llm.LLM 接口
type Anthropic struct {
	client anthropic.Client
	config llm.Config
}

func init() {
	llm.Register("anthropic", NewAnthropic)
}

// NewAnthropic 创建 Anthropic 实例
func NewAnthropic(config llm.Config) (llm.LLM, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("anthropic: API key is required")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(config.APIKey),
	}
	if config.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(config.BaseURL))
	}

	return &Anthropic{
		client: anthropic.NewClient(opts...),
		config: config,
	}, nil
}

// Generate 生成响应
func (a *Anthropic) Generate(ctx context.Context, messages []llm.Message, opts ...llm.Option) (*llm.Response, error) {
	// 应用选项
	options := &llm.Options{
		Model:       "claude-sonnet-4-6",
		Temperature: 1.0,
		MaxTokens:   4096,
	}
	for _, opt := range opts {
		opt(options)
	}

	// 转换消息格式
	antMessages := convertMessages(messages)

	// 构建请求
	req := anthropic.MessageNewParams{
		Model:     anthropic.Model(options.Model),
		MaxTokens: int64(options.MaxTokens),
		Messages:  antMessages,
	}

	// Temperature
	if options.Temperature > 0 {
		req.Temperature = anthropic.Float(float64(options.Temperature))
	}

	// 获取重试配置（默认使用 DefaultRetryConfig）
	retryConfig := llm.DefaultRetryConfig
	if options.Retry != nil {
		retryConfig = *options.Retry
	}

	// 使用重试机制调用 API
	resp, err := llm.DoWithRetry(ctx, retryConfig, func() (*anthropic.Message, error) {
		r, err := a.client.Messages.New(ctx, req)
		if err != nil {
			return nil, convertError(err)
		}
		return r, nil
	})

	if err != nil {
		return nil, err
	}

	// 解析响应
	return parseResponse(resp), nil
}

// Stream 流式生成响应
func (a *Anthropic) Stream(ctx context.Context, messages []llm.Message, opts ...llm.Option) (<-chan llm.Chunk, error) {
	// 应用选项
	options := &llm.Options{
		Model:       "claude-sonnet-4-6",
		Temperature: 1.0,
		MaxTokens:   4096,
	}
	for _, opt := range opts {
		opt(options)
	}

	// 转换消息格式
	antMessages := convertMessages(messages)

	// 构建请求
	req := anthropic.MessageNewParams{
		Model:     anthropic.Model(options.Model),
		MaxTokens: int64(options.MaxTokens),
		Messages:  antMessages,
	}

	// Temperature
	if options.Temperature > 0 {
		req.Temperature = anthropic.Float(float64(options.Temperature))
	}

	// 添加工具
	if len(options.Tools) > 0 {
		req.Tools = convertTools(options.Tools)
	}

	// 创建流
	stream := a.client.Messages.NewStreaming(ctx, req)

	// 创建 channel
	chunkCh := make(chan llm.Chunk, 10)

	// 启动 goroutine 读取流
	go func() {
		defer close(chunkCh)

		var usage llm.Usage
		var currentToolCall *llm.ToolCall

		for stream.Next() {
			event := stream.Current()

			switch event.Type {
			case "message_start":
				if event.Message.Usage.InputTokens > 0 {
					usage.InputTokens = int(event.Message.Usage.InputTokens)
				}

			case "content_block_start":
				chunkCh <- llm.Chunk{Type: "content_start"}

			case "content_block_delta":
				delta := event.Delta
				if delta.Type == "text_delta" {
					chunkCh <- llm.Chunk{
						Type: "text_delta",
						Text: delta.Text,
					}
				} else if delta.Type == "input_json_delta" {
					// 工具调用的输入正在流式传输
					// 暂时不处理，等到 content_block_stop
				}

			case "content_block_stop":
				// 如果有工具调用，发送
				if currentToolCall != nil {
					chunkCh <- llm.Chunk{
						Type:     "tool_use",
						ToolCall: currentToolCall,
					}
					currentToolCall = nil
				}

			case "message_delta":
				if event.Usage.OutputTokens > 0 {
					usage.OutputTokens = int(event.Usage.OutputTokens)
				}

			case "message_stop":
				chunkCh <- llm.Chunk{
					Type:  "done",
					Usage: &usage,
				}
			}
		}

		if err := stream.Err(); err != nil {
			chunkCh <- llm.Chunk{Type: "error", Error: convertError(err)}
		}
	}()

	return chunkCh, nil
}
