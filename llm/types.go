package llm

import "encoding/json"

// Message 表示一条对话消息
type Message struct {
	Role    string         `json:"role"`    // "user" | "assistant" | "system"
	Content []ContentBlock `json:"content"` // 支持多模态内容
}

// ContentBlock 表示消息中的一个内容块
type ContentBlock struct {
	Type       string      `json:"type"` // "text" | "image" | "tool_use" | "tool_result"
	Text       string      `json:"text,omitempty"`
	ImageData  []byte      `json:"image_data,omitempty"`
	MimeType   string      `json:"mime_type,omitempty"`
	ToolCall   *ToolCall   `json:"tool_call,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
}

// Response LLM 生成的响应
type Response struct {
	Content    []ContentBlock `json:"content"`     // 返回的内容（文本 + 工具调用）
	StopReason string         `json:"stop_reason"` // "end_turn" | "tool_use" | "max_tokens"
	Usage      Usage          `json:"usage"`       // Token 使用统计
}

// Chunk 流式响应的数据块
type Chunk struct {
	Type     string      `json:"type"` // "content_start" | "text_delta" | "tool_use" | "done"
	Text     string      `json:"text,omitempty"`
	ToolCall *ToolCall   `json:"tool_call,omitempty"`
	Usage    *Usage      `json:"usage,omitempty"`
	Error    error       `json:"error,omitempty"`
}

// Tool 工具定义
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema
}

// ToolCall LLM 发起的工具调用
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"` // 工具参数（JSON）
}

// ToolResult 工具执行结果
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
}

// Usage Token 使用统计
type Usage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`  // Anthropic 缓存命中
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"` // Anthropic 缓存写入
}

// Config LLM 初始化配置
type Config struct {
	APIKey  string
	BaseURL string // 可选，用于自定义 endpoint
}

// CacheConfig 缓存配置（Anthropic Prompt Caching）
type CacheConfig struct {
	Enabled             bool // 是否启用缓存
	CacheSystemPrompt   bool // 是否缓存 system prompt
	CachePrefixMessages int  // 缓存前 N 条消息
}
