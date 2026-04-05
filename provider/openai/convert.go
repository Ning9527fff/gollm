package openai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/Ning9527fff/gollm/llm"

	"github.com/sashabaranov/go-openai"
)

// convertMessages 将 llm.Message 转换为 OpenAI 格式
func convertMessages(messages []llm.Message) ([]openai.ChatCompletionMessage, error) {
	result := make([]openai.ChatCompletionMessage, 0, len(messages))

	for _, msg := range messages {
		oaiMsg := openai.ChatCompletionMessage{
			Role: msg.Role,
		}

		// 处理内容块
		if len(msg.Content) == 1 && msg.Content[0].Type == "text" {
			// 纯文本消息
			oaiMsg.Content = msg.Content[0].Text
		} else {
			// 多模态消息
			parts := make([]openai.ChatMessagePart, 0, len(msg.Content))
			for _, block := range msg.Content {
				switch block.Type {
				case "text":
					parts = append(parts, openai.ChatMessagePart{
						Type: openai.ChatMessagePartTypeText,
						Text: block.Text,
					})

				case "image":
					// 转换为 base64
					imageURL := fmt.Sprintf("data:%s;base64,%s",
						block.MimeType,
						base64.StdEncoding.EncodeToString(block.ImageData))
					parts = append(parts, openai.ChatMessagePart{
						Type: openai.ChatMessagePartTypeImageURL,
						ImageURL: &openai.ChatMessageImageURL{
							URL: imageURL,
						},
					})

				case "tool_use":
					// OpenAI 的工具调用在单独的字段中
					oaiMsg.ToolCalls = append(oaiMsg.ToolCalls, openai.ToolCall{
						ID:   block.ToolCall.ID,
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      block.ToolCall.Name,
							Arguments: string(block.ToolCall.Input),
						},
					})

				case "tool_result":
					// OpenAI 的工具结果作为单独的消息
					oaiMsg.Role = "tool"
					oaiMsg.Content = block.ToolResult.Content
					oaiMsg.ToolCallID = block.ToolResult.ToolUseID
				}
			}

			if len(parts) > 0 {
				oaiMsg.MultiContent = parts
			}
		}

		result = append(result, oaiMsg)
	}

	return result, nil
}

// convertTools 将 llm.Tool 转换为 OpenAI 格式
func convertTools(tools []llm.Tool) []openai.Tool {
	result := make([]openai.Tool, 0, len(tools))

	for _, tool := range tools {
		// 解析 JSON Schema
		var params map[string]any
		if err := json.Unmarshal(tool.InputSchema, &params); err != nil {
			continue
		}

		result = append(result, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  params,
			},
		})
	}

	return result
}

// parseResponse 解析 OpenAI 响应
func parseResponse(resp *openai.ChatCompletionResponse) *llm.Response {
	if len(resp.Choices) == 0 {
		return &llm.Response{
			Content:    []llm.ContentBlock{},
			StopReason: "unknown",
			Usage: llm.Usage{
				InputTokens:  resp.Usage.PromptTokens,
				OutputTokens: resp.Usage.CompletionTokens,
			},
		}
	}

	choice := resp.Choices[0]
	content := []llm.ContentBlock{}

	// 文本内容
	if choice.Message.Content != "" {
		content = append(content, llm.ContentBlock{
			Type: "text",
			Text: choice.Message.Content,
		})
	}

	// 工具调用
	for _, toolCall := range choice.Message.ToolCalls {
		content = append(content, llm.ContentBlock{
			Type: "tool_use",
			ToolCall: &llm.ToolCall{
				ID:    toolCall.ID,
				Name:  toolCall.Function.Name,
				Input: json.RawMessage(toolCall.Function.Arguments),
			},
		})
	}

	// 停止原因
	stopReason := string(choice.FinishReason)
	switch stopReason {
	case "tool_calls":
		stopReason = "tool_use"
	case "stop":
		stopReason = "end_turn"
	}

	return &llm.Response{
		Content:    content,
		StopReason: stopReason,
		Usage: llm.Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}
}

// parseStreamChunk 解析流式响应
func parseStreamChunk(chunk *openai.ChatCompletionStreamResponse) llm.Chunk {
	if len(chunk.Choices) == 0 {
		return llm.Chunk{Type: "unknown"}
	}

	delta := chunk.Choices[0].Delta

	// 文本增量
	if delta.Content != "" {
		return llm.Chunk{
			Type: "text_delta",
			Text: delta.Content,
		}
	}

	// 工具调用
	if len(delta.ToolCalls) > 0 {
		toolCall := delta.ToolCalls[0]
		return llm.Chunk{
			Type: "tool_use",
			ToolCall: &llm.ToolCall{
				ID:    toolCall.ID,
				Name:  toolCall.Function.Name,
				Input: json.RawMessage(toolCall.Function.Arguments),
			},
		}
	}

	// 结束
	if chunk.Choices[0].FinishReason != "" {
		usage := &llm.Usage{}
		if chunk.Usage != nil {
			usage.InputTokens = chunk.Usage.PromptTokens
			usage.OutputTokens = chunk.Usage.CompletionTokens
		}
		return llm.Chunk{
			Type:  "done",
			Usage: usage,
		}
	}

	return llm.Chunk{Type: "content_start"}
}

// convertError 将 OpenAI 错误转换为 LLMError
func convertError(err error) error {
	if err == nil {
		return nil
	}

	// 尝试解析为 OpenAI API 错误
	if apiErr, ok := err.(*openai.APIError); ok {
		return llm.NewLLMError(
			"openai",
			apiErr.HTTPStatusCode,
			apiErr.Message,
			err,
		)
	}

	// 尝试解析为 OpenAI RequestError
	if reqErr, ok := err.(*openai.RequestError); ok {
		return llm.NewLLMError(
			"openai",
			reqErr.HTTPStatusCode,
			reqErr.Err.Error(),
			err,
		)
	}

	// 其他错误，默认为不可重试
	return llm.NewLLMError(
		"openai",
		0,
		err.Error(),
		err,
	)
}
