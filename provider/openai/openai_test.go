package openai

import (
	"context"
	"goLLM/llm"
	"os"
	"testing"
)

func TestOpenAIGenerate(t *testing.T) {
	// 从环境变量读取配置
	apiKey := os.Getenv("OPENAI_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")

	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping test")
	}

	// 创建 OpenAI 客户端
	config := llm.Config{
		APIKey:  apiKey,
		BaseURL: baseURL,
	}

	client, err := NewOpenAI(config)
	if err != nil {
		t.Fatalf("Failed to create OpenAI client: %v", err)
	}

	// 构建消息
	messages := []llm.Message{
		{
			Role: "user",
			Content: []llm.ContentBlock{
				{
					Type: "text",
					Text: "Hello! Please respond with a short greeting.",
				},
			},
		},
	}

	// 发送请求（使用 NewAPI 支持的 Gemini 模型）
	ctx := context.Background()
	resp, err := client.Generate(ctx, messages,
		llm.WithModel("gemini-3.1-pro-preview"),
		llm.WithMaxTokens(100),
		llm.WithTemperature(0.7),
	)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// 验证响应
	if len(resp.Content) == 0 {
		t.Fatal("Response content is empty")
	}

	if resp.Content[0].Type != "text" {
		t.Fatalf("Expected text content, got %s", resp.Content[0].Type)
	}

	if resp.Content[0].Text == "" {
		t.Fatal("Response text is empty")
	}

	// 打印结果
	t.Logf("Response: %s", resp.Content[0].Text)
	t.Logf("Stop Reason: %s", resp.StopReason)
	t.Logf("Usage - Input: %d, Output: %d tokens",
		resp.Usage.InputTokens, resp.Usage.OutputTokens)
}
