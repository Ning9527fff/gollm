package gemini

import (
	"context"
	"encoding/json"
	"goLLM/llm"
	"os"
	"testing"
)

func TestGeminiToolCalling(t *testing.T) {
	// 从环境变量读取配置
	apiKey := os.Getenv("GEMINI_API_KEY")
	baseURL := os.Getenv("GEMINI_BASE_URL")

	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set, skipping test")
	}

	// 创建客户端
	config := llm.Config{
		APIKey:  apiKey,
		BaseURL: baseURL,
	}

	client, err := NewGemini(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// 定义工具：获取天气
	weatherTool := llm.Tool{
		Name:        "get_weather",
		Description: "Get the current weather for a city",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"city": {
					"type": "string",
					"description": "The city name"
				}
			},
			"required": ["city"]
		}`),
	}

	// 第一轮：用户请求 + LLM 调用工具
	messages := []llm.Message{
		{
			Role: "user",
			Content: []llm.ContentBlock{
				{Type: "text", Text: "What's the weather in Paris?"},
			},
		},
	}

	ctx := context.Background()
	resp1, err := client.Generate(ctx, messages,
		llm.WithModel("gemini-3.1-pro-preview"),
		llm.WithTools([]llm.Tool{weatherTool}),
		llm.WithMaxTokens(1000),
	)

	if err != nil {
		t.Fatalf("First round failed: %v", err)
	}

	t.Logf("First response - StopReason: %s", resp1.StopReason)
	t.Logf("Content blocks: %d", len(resp1.Content))

	// 验证 LLM 是否调用了工具
	var toolCall *llm.ToolCall
	for _, block := range resp1.Content {
		t.Logf("Block type: %s", block.Type)
		if block.Type == "tool_use" {
			toolCall = block.ToolCall
			t.Logf("Tool called: %s", toolCall.Name)
			t.Logf("Tool args: %s", string(toolCall.Input))
			break
		}
	}

	if toolCall == nil {
		t.Fatal("Expected LLM to call get_weather tool, but no tool call found")
	}

	if toolCall.Name != "get_weather" {
		t.Fatalf("Expected tool name 'get_weather', got '%s'", toolCall.Name)
	}

	// 解析工具参数
	var args struct {
		City string `json:"city"`
	}
	if err := json.Unmarshal(toolCall.Input, &args); err != nil {
		t.Fatalf("Failed to parse tool arguments: %v", err)
	}

	t.Logf("Parsed city: %s", args.City)

	// 模拟执行工具
	weatherResult := "Paris: 22°C, Sunny"

	// 第二轮：返回工具结果 + LLM 生成最终答案
	messages = append(messages,
		llm.Message{
			Role:    "assistant",
			Content: resp1.Content, // 包含 tool_use (FunctionCall)
		},
		llm.Message{
			Role: "user", // Gemini 的 FunctionResponse 也作为 user 消息
			Content: []llm.ContentBlock{
				{
					Type: "tool_result",
					ToolResult: &llm.ToolResult{
						ToolUseID: toolCall.Name, // Gemini 使用函数名作为 ID
						Content:   weatherResult,
						IsError:   false,
					},
				},
			},
		},
	)

	resp2, err := client.Generate(ctx, messages,
		llm.WithModel("gemini-3.1-pro-preview"),
		llm.WithTools([]llm.Tool{weatherTool}),
		llm.WithMaxTokens(1000),
	)

	if err != nil {
		t.Fatalf("Second round failed: %v", err)
	}

	t.Logf("Second response - StopReason: %s", resp2.StopReason)

	// 验证最终答案包含天气信息
	var finalText string
	for _, block := range resp2.Content {
		if block.Type == "text" {
			finalText = block.Text
			break
		}
	}

	if finalText == "" {
		t.Fatal("Expected final answer in text, got empty")
	}

	t.Logf("Final answer: %s", finalText)
	t.Logf("Total usage - Round 1: %d+%d, Round 2: %d+%d tokens",
		resp1.Usage.InputTokens, resp1.Usage.OutputTokens,
		resp2.Usage.InputTokens, resp2.Usage.OutputTokens)
}
