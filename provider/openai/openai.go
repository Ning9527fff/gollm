package openai

import (
	"context"
	"fmt"

	"goLLM/llm"

	"github.com/sashabaranov/go-openai"
)

// OpenAI 实现 llm.LLM 接口
type OpenAI struct {
	client *openai.Client
	config llm.Config
}

func init() {
	llm.Register("openai", NewOpenAI)
}

// NewOpenAI 创建 OpenAI 实例
func NewOpenAI(config llm.Config) (llm.LLM, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("openai: API key is required")
	}

	clientConfig := openai.DefaultConfig(config.APIKey)
	if config.BaseURL != "" {
		clientConfig.BaseURL = config.BaseURL
	}

	return &OpenAI{
		client: openai.NewClientWithConfig(clientConfig),
		config: config,
	}, nil
}

// Generate 生成响应
func (o *OpenAI) Generate(ctx context.Context, messages []llm.Message, opts ...llm.Option) (*llm.Response, error) {
	// 应用选项
	options := &llm.Options{
		Model:       "gpt-4o",
		Temperature: 1.0,
		MaxTokens:   4096,
	}
	for _, opt := range opts {
		opt(options)
	}

	// 转换消息格式
	oaiMessages, err := convertMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("openai: convert messages: %w", err)
	}

	// 构建请求
	req := openai.ChatCompletionRequest{
		Model:       options.Model,
		Messages:    oaiMessages,
		Temperature: options.Temperature,
		MaxTokens:   options.MaxTokens,
	}

	// 添加工具
	if len(options.Tools) > 0 {
		req.Tools = convertTools(options.Tools)
	}

	// 调用 API
	resp, err := o.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai: api call: %w", err)
	}

	// 解析响应
	return parseResponse(&resp), nil
}

// Stream 流式生成响应
func (o *OpenAI) Stream(ctx context.Context, messages []llm.Message, opts ...llm.Option) (<-chan llm.Chunk, error) {
	// 应用选项
	options := &llm.Options{
		Model:       "gpt-4o",
		Temperature: 1.0,
		MaxTokens:   4096,
	}
	for _, opt := range opts {
		opt(options)
	}

	// 转换消息格式
	oaiMessages, err := convertMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("openai: convert messages: %w", err)
	}

	// 构建请求
	req := openai.ChatCompletionRequest{
		Model:       options.Model,
		Messages:    oaiMessages,
		Temperature: options.Temperature,
		MaxTokens:   options.MaxTokens,
		Stream:      true,
	}

	// 添加工具
	if len(options.Tools) > 0 {
		req.Tools = convertTools(options.Tools)
	}

	// 创建流
	stream, err := o.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("openai: create stream: %w", err)
	}

	// 创建 channel
	chunkCh := make(chan llm.Chunk)

	// 启动 goroutine 读取流
	go func() {
		defer close(chunkCh)
		defer stream.Close()

		for {
			chunk, err := stream.Recv()
			if err != nil {
				// EOF 表示正常结束
				if err.Error() == "EOF" {
					chunkCh <- llm.Chunk{Type: "done"}
					return
				}
				chunkCh <- llm.Chunk{Type: "error", Error: err}
				return
			}

			// 解析流式响应
			streamChunk := parseStreamChunk(&chunk)
			chunkCh <- streamChunk
		}
	}()

	return chunkCh, nil
}
