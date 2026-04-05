package llm

import (
	"fmt"
	"net/http"
)

// LLMError 是所有 LLM 相关错误的统一类型
type LLMError struct {
	Provider   string // 提供商名称 (openai, anthropic, gemini)
	StatusCode int    // HTTP 状态码 (如果适用)
	Message    string // 错误消息
	Retryable  bool   // 是否可重试
	Err        error  // 原始错误
}

func (e *LLMError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("[%s] %s (status: %d)", e.Provider, e.Message, e.StatusCode)
	}
	return fmt.Sprintf("[%s] %s", e.Provider, e.Message)
}

func (e *LLMError) Unwrap() error {
	return e.Err
}

// NewLLMError 创建一个新的 LLM 错误
func NewLLMError(provider string, statusCode int, message string, err error) *LLMError {
	return &LLMError{
		Provider:   provider,
		StatusCode: statusCode,
		Message:    message,
		Retryable:  isRetryableError(statusCode, err),
		Err:        err,
	}
}

// isRetryableError 判断错误是否可重试
func isRetryableError(statusCode int, err error) bool {
	if err == nil && statusCode == 0 {
		return false
	}

	// 基于 HTTP 状态码判断
	switch statusCode {
	case http.StatusTooManyRequests, // 429 - 速率限制
		http.StatusRequestTimeout,        // 408 - 请求超时
		http.StatusInternalServerError,   // 500 - 服务器错误
		http.StatusBadGateway,            // 502 - 网关错误
		http.StatusServiceUnavailable,    // 503 - 服务不可用
		http.StatusGatewayTimeout:        // 504 - 网关超时
		return true

	case http.StatusBadRequest,   // 400 - 客户端错误 (不可重试)
		http.StatusUnauthorized,      // 401 - 未授权 (不可重试)
		http.StatusForbidden,         // 403 - 禁止访问 (不可重试)
		http.StatusNotFound,          // 404 - 未找到 (不可重试)
		http.StatusUnprocessableEntity: // 422 - 无法处理 (不可重试)
		return false
	}

	// 网络错误通常可重试
	if err != nil {
		errMsg := err.Error()
		// 常见的网络错误关键词
		retryableKeywords := []string{
			"timeout",
			"connection refused",
			"connection reset",
			"EOF",
			"network is unreachable",
			"temporary failure",
		}

		for _, keyword := range retryableKeywords {
			if contains(errMsg, keyword) {
				return true
			}
		}
	}

	// 默认不重试
	return false
}

// contains 检查字符串是否包含子串（不区分大小写）
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalFold(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalFold(s1, s2 string) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := 0; i < len(s1); i++ {
		c1, c2 := s1[i], s2[i]
		if c1 != c2 {
			// 简单的大小写转换
			if c1 >= 'A' && c1 <= 'Z' {
				c1 += 'a' - 'A'
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 += 'a' - 'A'
			}
			if c1 != c2 {
				return false
			}
		}
	}
	return true
}
