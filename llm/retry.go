package llm

import (
	"context"
	"math"
	"math/rand"
	"strconv"
	"time"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries   int           // 最大重试次数 (0 表示不重试)
	InitialDelay time.Duration // 初始延迟
	MaxDelay     time.Duration // 最大延迟
	Multiplier   float64       // 退避倍数
}

// DefaultRetryConfig 默认重试配置
var DefaultRetryConfig = RetryConfig{
	MaxRetries:   3,
	InitialDelay: 500 * time.Millisecond,
	MaxDelay:     30 * time.Second,
	Multiplier:   2.0,
}

// NoRetry 不重试配置
var NoRetry = RetryConfig{
	MaxRetries: 0,
}

// DoWithRetry 包装函数使其支持重试（导出供 provider 使用）
func DoWithRetry[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// 执行函数
		result, lastErr = fn()

		// 成功则返回
		if lastErr == nil {
			return result, nil
		}

		// 检查是否可重试
		if !isRetryable(lastErr) {
			return result, lastErr
		}

		// 最后一次尝试失败，不再重试
		if attempt >= cfg.MaxRetries {
			return result, lastErr
		}

		// 计算延迟时间
		delay := calculateDelay(cfg, attempt, lastErr)

		// 等待，支持 context 取消
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(delay):
			// 继续重试
		}
	}

	return result, lastErr
}

// isRetryable 判断错误是否可重试
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// 如果是 LLMError，使用其 Retryable 字段
	if llmErr, ok := err.(*LLMError); ok {
		return llmErr.Retryable
	}

	// 默认不重试
	return false
}

// calculateDelay 计算延迟时间（指数退避 + 随机抖动）
func calculateDelay(cfg RetryConfig, attempt int, err error) time.Duration {
	// 检查是否有 Retry-After 响应头
	if llmErr, ok := err.(*LLMError); ok {
		if retryAfter := parseRetryAfter(llmErr); retryAfter > 0 {
			// 遵守 Retry-After，但不超过 MaxDelay
			if retryAfter > cfg.MaxDelay {
				return cfg.MaxDelay
			}
			return retryAfter
		}
	}

	// 指数退避: delay = InitialDelay * (Multiplier ^ attempt)
	delay := float64(cfg.InitialDelay) * math.Pow(cfg.Multiplier, float64(attempt))

	// 限制最大延迟
	if delay > float64(cfg.MaxDelay) {
		delay = float64(cfg.MaxDelay)
	}

	// 添加随机抖动 (±25%)
	jitter := delay * 0.25 * (rand.Float64()*2 - 1)
	delay += jitter

	// 确保非负
	if delay < 0 {
		delay = float64(cfg.InitialDelay)
	}

	return time.Duration(delay)
}

// parseRetryAfter 解析 Retry-After 头（如果有）
func parseRetryAfter(err *LLMError) time.Duration {
	// 注: 当前 LLMError 没有存储响应头
	// 这里留作未来扩展，Provider 可以在错误中携带 Retry-After 信息
	// 可以通过扩展 LLMError 添加 Headers 字段实现

	// 临时实现：如果是 429 错误，返回建议的重试时间
	if err.StatusCode == 429 {
		// 速率限制，建议等待 1-5 秒
		return time.Duration(1+rand.Intn(4)) * time.Second
	}

	return 0
}

// parseRetryAfterHeader 解析 Retry-After 响应头
// retryAfter 可以是秒数 (整数) 或 HTTP 日期
func parseRetryAfterHeader(retryAfter string) time.Duration {
	if retryAfter == "" {
		return 0
	}

	// 尝试解析为秒数
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	// 尝试解析为 HTTP 日期格式
	if t, err := time.Parse(time.RFC1123, retryAfter); err == nil {
		duration := time.Until(t)
		if duration > 0 {
			return duration
		}
	}

	return 0
}
