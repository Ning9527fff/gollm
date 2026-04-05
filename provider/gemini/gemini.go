package gemini

import (
	"context"
	"encoding/json"
	"fmt"

	"goLLM/llm"

	"google.golang.org/genai"
)

// Gemini 实现 llm.LLM 接口
type Gemini struct {
	client *genai.Client
	config llm.Config
}

func init() {
	llm.Register("gemini", NewGemini)
}

// NewGemini 创建 Gemini 实例
func NewGemini(config llm.Config) (llm.LLM, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("gemini: API key is required")
	}

	ctx := context.Background()
	clientConfig := &genai.ClientConfig{
		APIKey:  config.APIKey,
		Backend: genai.BackendGeminiAPI,
	}

	// 自定义 BaseURL（如果有）
	if config.BaseURL != "" {
		clientConfig.HTTPOptions = genai.HTTPOptions{
			BaseURL: config.BaseURL,
		}
	}

	client, err := genai.NewClient(ctx, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("gemini: create client: %w", err)
	}

	return &Gemini{
		client: client,
		config: config,
	}, nil
}

// Generate 生成响应
func (g *Gemini) Generate(ctx context.Context, messages []llm.Message, opts ...llm.Option) (*llm.Response, error) {
	// 应用选项
	options := &llm.Options{
		Model:       "gemini-2.0-flash-exp",
		Temperature: 1.0,
		MaxTokens:   8192,
	}
	for _, opt := range opts {
		opt(options)
	}

	// 转换消息格式
	systemPrompt, geminiMessages := convertMessages(messages)

	// 构建请求配置
	reqConfig := &genai.GenerateContentConfig{}

	// System Instruction
	if systemPrompt != "" {
		reqConfig.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{
				{Text: systemPrompt},
			},
		}
	}

	// Temperature
	if options.Temperature > 0 {
		reqConfig.Temperature = &options.Temperature
	}

	// MaxTokens
	if options.MaxTokens > 0 {
		reqConfig.MaxOutputTokens = int32(options.MaxTokens)
	}

	// 添加工具
	if len(options.Tools) > 0 {
		reqConfig.Tools = convertTools(options.Tools)
	}

	// 调用 API
	resp, err := g.client.Models.GenerateContent(
		ctx,
		options.Model,
		geminiMessages,
		reqConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("gemini: api call: %w", err)
	}

	// 解析响应
	return parseResponse(resp), nil
}

// Stream 流式生成响应
func (g *Gemini) Stream(ctx context.Context, messages []llm.Message, opts ...llm.Option) (<-chan llm.Chunk, error) {
	// 应用选项
	options := &llm.Options{
		Model:       "gemini-2.0-flash-exp",
		Temperature: 1.0,
		MaxTokens:   8192,
	}
	for _, opt := range opts {
		opt(options)
	}

	// 转换消息格式
	systemPrompt, geminiMessages := convertMessages(messages)

	// 构建请求配置
	reqConfig := &genai.GenerateContentConfig{}

	// System Instruction
	if systemPrompt != "" {
		reqConfig.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{
				{Text: systemPrompt},
			},
		}
	}

	// Temperature
	if options.Temperature > 0 {
		reqConfig.Temperature = &options.Temperature
	}

	// MaxTokens
	if options.MaxTokens > 0 {
		reqConfig.MaxOutputTokens = int32(options.MaxTokens)
	}

	// 添加工具
	if len(options.Tools) > 0 {
		reqConfig.Tools = convertTools(options.Tools)
	}

	// 创建 channel
	chunkCh := make(chan llm.Chunk, 10)

	// 启动 goroutine 调用流式 API
	go func() {
		defer close(chunkCh)

		var totalUsage llm.Usage

		// 调用流式 API (iter.Seq2)
		for resp, err := range g.client.Models.GenerateContentStream(
			ctx,
			options.Model,
			geminiMessages,
			reqConfig,
		) {
			if err != nil {
				chunkCh <- llm.Chunk{Type: "error", Error: err}
				return
			}

			// 处理每个流式响应
			if len(resp.Candidates) > 0 {
				candidate := resp.Candidates[0]

				if candidate.Content != nil {
					for _, part := range candidate.Content.Parts {
						// 文本流式输出
						if part.Text != "" {
							chunkCh <- llm.Chunk{
								Type: "text_delta",
								Text: part.Text,
							}
						}

						// 函数调用（通常在最后一个 chunk）
						if part.FunctionCall != nil {
							chunkCh <- llm.Chunk{
								Type: "tool_use",
								ToolCall: &llm.ToolCall{
									ID:   part.FunctionCall.Name,
									Name: part.FunctionCall.Name,
									Input: func() []byte {
										data, _ := json.Marshal(part.FunctionCall.Args)
										return data
									}(),
								},
							}
						}
					}
				}
			}

			// 累积 token 使用量
			if resp.UsageMetadata != nil {
				totalUsage.InputTokens = int(resp.UsageMetadata.PromptTokenCount)
				totalUsage.OutputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
			}
		}

		// 发送完成信号
		chunkCh <- llm.Chunk{
			Type:  "done",
			Usage: &totalUsage,
		}
	}()

	return chunkCh, nil
}
