package gemini

import (
	"encoding/json"
	"goLLM/llm"

	"google.golang.org/genai"
)

// convertMessages 将 llm.Message 转换为 Gemini 格式
// 返回：system prompt, messages
func convertMessages(messages []llm.Message) (string, []*genai.Content) {
	var systemPrompt string
	result := make([]*genai.Content, 0)

	for _, msg := range messages {
		// 提取 system prompt
		if msg.Role == "system" {
			for _, block := range msg.Content {
				if block.Type == "text" {
					systemPrompt += block.Text
				}
			}
			continue
		}

		// 转换消息内容
		parts := make([]*genai.Part, 0, len(msg.Content))

		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				parts = append(parts, &genai.Part{
					Text: block.Text,
				})

			case "image":
				// 图片数据
				parts = append(parts, &genai.Part{
					InlineData: &genai.Blob{
						MIMEType: block.MimeType,
						Data:     block.ImageData,
					},
				})

			case "tool_use":
				// Gemini 的函数调用（作为 assistant 消息返回）
				if block.ToolCall != nil {
					var argsMap map[string]any
					json.Unmarshal(block.ToolCall.Input, &argsMap)
					parts = append(parts, &genai.Part{
						FunctionCall: &genai.FunctionCall{
							Name: block.ToolCall.Name,
							Args: argsMap,
						},
					})
				}

			case "tool_result":
				// Gemini 的函数响应（作为 function 角色的消息）
				if block.ToolResult != nil {
					parts = append(parts, &genai.Part{
						FunctionResponse: &genai.FunctionResponse{
							Name: block.ToolResult.ToolUseID, // Gemini 使用函数名作为标识
							Response: map[string]any{
								"result": block.ToolResult.Content,
							},
						},
					})
				}
			}
		}

		// 创建消息
		content := &genai.Content{
			Parts: parts,
		}

		// 设置角色
		switch msg.Role {
		case "user":
			content.Role = genai.RoleUser
		case "assistant":
			content.Role = genai.RoleModel
		}

		result = append(result, content)
	}

	return systemPrompt, result
}

// convertTools 将 llm.Tool 转换为 Gemini 格式
func convertTools(tools []llm.Tool) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}

	functionDeclarations := make([]*genai.FunctionDeclaration, 0, len(tools))

	for _, tool := range tools {
		// 解析 JSON Schema
		var schema map[string]any
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			continue
		}

		functionDeclarations = append(functionDeclarations, &genai.FunctionDeclaration{
			Name:                 tool.Name,
			Description:          tool.Description,
			ParametersJsonSchema: schema,
		})
	}

	return []*genai.Tool{
		{
			FunctionDeclarations: functionDeclarations,
		},
	}
}

// parseResponse 解析 Gemini 响应
func parseResponse(resp *genai.GenerateContentResponse) *llm.Response {
	content := make([]llm.ContentBlock, 0)

	// 检查是否有候选结果
	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]

		// 遍历内容部分
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				// 文本内容
				if part.Text != "" {
					content = append(content, llm.ContentBlock{
						Type: "text",
						Text: part.Text,
					})
				}

				// 函数调用
				if part.FunctionCall != nil {
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)
					content = append(content, llm.ContentBlock{
						Type: "tool_use",
						ToolCall: &llm.ToolCall{
							ID:    part.FunctionCall.Name, // Gemini 使用函数名作为 ID
							Name:  part.FunctionCall.Name,
							Input: argsJSON,
						},
					})
				}
			}
		}
	}

	// 停止原因
	stopReason := "end_turn"
	if len(resp.Candidates) > 0 {
		finishReason := resp.Candidates[0].FinishReason
		switch finishReason {
		case genai.FinishReasonStop:
			// 检查是否有函数调用
			hasFunctionCall := false
			for _, block := range content {
				if block.Type == "tool_use" {
					hasFunctionCall = true
					break
				}
			}
			if hasFunctionCall {
				stopReason = "tool_use"
			} else {
				stopReason = "end_turn"
			}
		case genai.FinishReasonMaxTokens:
			stopReason = "max_tokens"
		}
	}

	// Token 使用统计
	usage := llm.Usage{}
	if resp.UsageMetadata != nil {
		usage.InputTokens = int(resp.UsageMetadata.PromptTokenCount)
		usage.OutputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	return &llm.Response{
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
	}
}

// convertError 将 Gemini 错误转换为 LLMError
func convertError(err error) error {
	if err == nil {
		return nil
	}

	// Gemini SDK 的错误类型处理
	// genai 包的错误类型可能不同，这里提供通用处理
	// 可以根据实际 SDK 错误类型进行更精确的处理

	return llm.NewLLMError(
		"gemini",
		0, // Gemini SDK 可能不暴露状态码
		err.Error(),
		err,
	)
}
