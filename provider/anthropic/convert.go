package anthropic

import (
	"encoding/json"
	"github.com/Ning9527fff/gollm/llm"

	"github.com/anthropics/anthropic-sdk-go"
)

// convertMessages 将 llm.Message 转换为 Anthropic 格式
func convertMessages(messages []llm.Message) []anthropic.MessageParam {
	result := make([]anthropic.MessageParam, 0)

	for _, msg := range messages {
		// 跳过 system 消息（Anthropic 需要单独处理）
		if msg.Role == "system" {
			continue
		}

		// 构建内容块
		contentBlocks := make([]anthropic.ContentBlockParamUnion, 0)

		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				contentBlocks = append(contentBlocks,
					anthropic.NewTextBlock(block.Text),
				)

			case "tool_use":
				// Anthropic 的工具调用
				// 解析 Input 为 map
				var inputMap map[string]any
				if err := json.Unmarshal(block.ToolCall.Input, &inputMap); err == nil {
					contentBlocks = append(contentBlocks,
						anthropic.NewToolUseBlock(
							block.ToolCall.ID,
							inputMap,
							block.ToolCall.Name,
						),
					)
				}

			case "tool_result":
				// Anthropic 的工具结果
				contentBlocks = append(contentBlocks,
					anthropic.NewToolResultBlock(
						block.ToolResult.ToolUseID,
						block.ToolResult.Content,
						block.ToolResult.IsError,
					),
				)
			}
		}

		// 创建消息
		if len(contentBlocks) > 0 {
			switch msg.Role {
			case "user":
				result = append(result, anthropic.NewUserMessage(contentBlocks...))
			case "assistant":
				result = append(result, anthropic.NewAssistantMessage(contentBlocks...))
			}
		}
	}

	return result
}

// convertTools 将 llm.Tool 转换为 Anthropic 格式
func convertTools(tools []llm.Tool) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, 0, len(tools))

	for _, tool := range tools {
		// 解析 JSON Schema
		var schemaMap map[string]any
		if err := json.Unmarshal(tool.InputSchema, &schemaMap); err != nil {
			continue
		}

		// 提取 properties 和 required
		properties, _ := schemaMap["properties"]
		required, _ := schemaMap["required"].([]any)

		// 转换 required 为 []string
		requiredStrs := make([]string, 0)
		for _, r := range required {
			if str, ok := r.(string); ok {
				requiredStrs = append(requiredStrs, str)
			}
		}

		// 构造 ToolParam
		toolParam := anthropic.ToolParam{
			Name: tool.Name,
			Description: anthropic.String(tool.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: properties,
				Required:   requiredStrs,
			},
		}

		result = append(result, anthropic.ToolUnionParam{
			OfTool: &toolParam,
		})
	}

	return result
}

// parseResponse 解析 Anthropic 响应
func parseResponse(resp *anthropic.Message) *llm.Response {
	content := make([]llm.ContentBlock, 0, len(resp.Content))

	// 解析内容块
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			content = append(content, llm.ContentBlock{
				Type: "text",
				Text: block.Text,
			})

		case "tool_use":
			// 工具调用
			content = append(content, llm.ContentBlock{
				Type: "tool_use",
				ToolCall: &llm.ToolCall{
					ID:    block.ID,
					Name:  block.Name,
					Input: block.Input,
				},
			})
		}
	}

	// 停止原因
	stopReason := "end_turn"
	if resp.StopReason != "" {
		stopReason = string(resp.StopReason)
	}

	return &llm.Response{
		Content:    content,
		StopReason: stopReason,
		Usage: llm.Usage{
			InputTokens:  int(resp.Usage.InputTokens),
			OutputTokens: int(resp.Usage.OutputTokens),
		},
	}
}

// convertError 将 Anthropic 错误转换为 LLMError
func convertError(err error) error {
	if err == nil {
		return nil
	}

	// Anthropic SDK 的错误类型处理
	// 注：anthropic-sdk-go 的错误类型可能不同，这里提供通用处理
	// 可以根据实际 SDK 错误类型进行更精确的处理

	return llm.NewLLMError(
		"anthropic",
		0, // Anthropic SDK 可能不暴露状态码
		err.Error(),
		err,
	)
}
