# go-llm

> 轻量级 Go LLM 编排库 - 统一接口，多模型支持

[![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.22-blue)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**go-llm** 是一个轻量级的 Go 语言 LLM（大语言模型）编排库，提供统一的接口来调用不同的 LLM 提供商，支持工具调用、流式输出等高级特性。

## ✨ 特性

- 🎯 **统一接口** - 一套代码适配 OpenAI、Anthropic、Gemini
- 🔧 **工具调用** - 完整的 Function Calling 支持
- 📡 **流式输出** - 实时流式文本生成
- 🖼️ **多模态** - 支持文本 + 图片（Gemini）
- 🔄 **自动重试** - 指数退避 + 错误分类，生产环境可靠
- ⚠️ **错误处理** - 结构化错误类型，支持错误判断和处理
- ⚙️ **灵活配置** - 环境变量 + JSON 配置文件
- 🚀 **易扩展** - 工厂模式，新Provider支持自动接入

## 📦 支持的 Provider

| Provider | 模型示例 | Generate | Stream | Tools | 多模态 |
|----------|---------|----------|--------|-------|--------|
| OpenAI | gpt-4o, gpt-4-turbo | ✅ | ✅ | ✅ | 🚧 |
| Anthropic | claude-sonnet-4-6, claude-opus-4-6 | ✅ | ✅ | ✅ | 🚧 |
| Gemini | gemini-2.0-flash-exp, gemini-1.5-pro | ✅ | ✅ | ✅ | ✅ |

## 🚀 快速开始

### 安装

```bash
go get github.com/yourusername/go-llm
```

### 基础使用

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "goLLM/config"
    "goLLM/llm"
    
    _ "goLLM/provider/openai"
)

func main() {
    // 1. 加载配置（自动从环境变量或配置文件读取）
    cfg, err := config.LoadConfig("openai")
    if err != nil {
        log.Fatal(err)
    }
    
    // 2. 创建 LLM 客户端
    client, err := llm.NewLLM("openai", *cfg)
    if err != nil {
        log.Fatal(err)
    }
    
    // 3. 构建消息
    messages := []llm.Message{
        {
            Role: "user",
            Content: []llm.ContentBlock{
                {Type: "text", Text: "Hello! How are you?"},
            },
        },
    }
    
    // 4. 调用 LLM
    resp, err := client.Generate(context.Background(), messages,
        llm.WithModel("gpt-4o"),
        llm.WithMaxTokens(100),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // 5. 获取响应
    fmt.Println(resp.Content[0].Text)
}
```

## 📖 使用示例

### 1. 流式输出

```go
// 创建流式请求
chunkCh, err := client.Stream(ctx, messages,
    llm.WithModel("gpt-4o"),
)

// 逐块读取响应
for chunk := range chunkCh {
    switch chunk.Type {
    case "text_delta":
        fmt.Print(chunk.Text)  // 实时输出
    case "done":
        fmt.Printf("\nTokens: %d + %d\n", 
            chunk.Usage.InputTokens, 
            chunk.Usage.OutputTokens)
    case "error":
        log.Fatal(chunk.Error)
    }
}
```

### 2. 工具调用（Function Calling）

```go
// 定义工具
weatherTool := llm.Tool{
    Name:        "get_weather",
    Description: "Get current weather for a city",
    InputSchema: json.RawMessage(`{
        "type": "object",
        "properties": {
            "city": {"type": "string"}
        },
        "required": ["city"]
    }`),
}

// 第一轮：LLM 调用工具
resp1, _ := client.Generate(ctx, messages,
    llm.WithTools([]llm.Tool{weatherTool}),
)

if resp1.StopReason == "tool_use" {
    // 执行工具
    for _, block := range resp1.Content {
        if block.Type == "tool_use" {
            result := executeWeatherTool(block.ToolCall)
            
            // 第二轮：返回工具结果
            messages = append(messages,
                llm.Message{Role: "assistant", Content: resp1.Content},
                llm.Message{Role: "user", Content: []llm.ContentBlock{
                    {
                        Type: "tool_result",
                        ToolResult: &llm.ToolResult{
                            ToolUseID: block.ToolCall.ID,
                            Content:   result,
                        },
                    },
                }},
            )
        }
    }
    
    // 获取最终答案
    resp2, _ := client.Generate(ctx, messages, llm.WithTools([]llm.Tool{weatherTool}))
    fmt.Println(resp2.Content[0].Text)
}
```

### 3. 多模态（图片 + 文本）

```go
imageData, _ := os.ReadFile("image.jpg")

messages := []llm.Message{
    {
        Role: "user",
        Content: []llm.ContentBlock{
            {Type: "text", Text: "What's in this image?"},
            {
                Type:      "image",
                ImageData: imageData,
                MimeType:  "image/jpeg",
            },
        },
    },
}

resp, _ := client.Generate(ctx, messages, llm.WithModel("gemini-2.0-flash-exp"))
```

### 4. 错误处理和重试

```go
// 使用默认重试配置（最多重试 3 次）
resp, err := client.Generate(ctx, messages,
    llm.WithModel("gpt-4o"),
    llm.WithDefaultRetry(),
)

// 自定义重试配置
customRetry := llm.RetryConfig{
    MaxRetries:   5,
    InitialDelay: 1 * time.Second,
    MaxDelay:     30 * time.Second,
    Multiplier:   2.0,
}

resp, err := client.Generate(ctx, messages,
    llm.WithModel("gpt-4o"),
    llm.WithRetry(customRetry),
)

// 禁用重试
resp, err := client.Generate(ctx, messages,
    llm.WithModel("gpt-4o"),
    llm.WithNoRetry(),
)

// 错误处理
if err != nil {
    if llmErr, ok := err.(*llm.LLMError); ok {
        fmt.Printf("Provider: %s\n", llmErr.Provider)
        fmt.Printf("Status: %d\n", llmErr.StatusCode)
        fmt.Printf("Message: %s\n", llmErr.Message)
        fmt.Printf("Retryable: %v\n", llmErr.Retryable)
    }
    log.Fatal(err)
}
```

## ⚙️ 配置

### 方式 1: 环境变量（推荐）

```bash
# OpenAI
export OPENAI_API_KEY="sk-..."
export OPENAI_BASE_URL="https://api.openai.com/v1"  # 可选

# Anthropic
export ANTHROPIC_API_KEY="sk-ant-..."

# Gemini
export GEMINI_API_KEY="..."
```

### 方式 2: JSON 配置文件

创建 `llm-config.json`:

```json
{
  "providers": {
    "openai": {
      "api_key": "sk-...",
      "base_url": "https://api.openai.com/v1"
    },
    "anthropic": {
      "api_key": "sk-ant-..."
    },
    "gemini": {
      "api_key": "..."
    }
  }
}
```

### 配置优先级

环境变量 > 指定配置文件 > 默认配置文件 (`./llm-config.json`)

## 🔧 API 参考

### 核心接口

```go
type LLM interface {
    Generate(ctx context.Context, messages []Message, opts ...Option) (*Response, error)
    Stream(ctx context.Context, messages []Message, opts ...Option) (<-chan Chunk, error)
}
```

### 选项函数

```go
llm.WithModel(model string)           // 设置模型
llm.WithTemperature(temp float32)     // 设置温度 [0, 2]
llm.WithMaxTokens(max int)            // 设置最大输出 tokens
llm.WithTools(tools []Tool)           // 添加工具
llm.WithCache(config CacheConfig)     // 启用缓存（仅 Anthropic）
llm.WithRetry(config RetryConfig)     // 自定义重试配置
llm.WithDefaultRetry()                // 使用默认重试配置
llm.WithNoRetry()                     // 禁用重试
```

### 消息结构

```go
type Message struct {
    Role    string         // "user" | "assistant" | "system"
    Content []ContentBlock
}

type ContentBlock struct {
    Type       string  // "text" | "image" | "tool_use" | "tool_result"
    Text       string
    ImageData  []byte
    MimeType   string
    ToolCall   *ToolCall
    ToolResult *ToolResult
}
```

### 响应结构

```go
type Response struct {
    Content    []ContentBlock
    StopReason string  // "end_turn" | "tool_use" | "max_tokens"
    Usage      Usage
}

type Usage struct {
    InputTokens      int
    OutputTokens     int
    CacheReadTokens  int  // Anthropic only
    CacheWriteTokens int  // Anthropic only
}
```


## 🎯 设计思想
1. **接口统一** - 一套接口适配所有 Provider
2. **渐进增强** - 基础功能必选，高级功能可选
3. **安全第一** - 工具调用需用户显式执行
4. **易于扩展** - 工厂模式，新增 Provider 无需改核心

## 📝 完整示例

查看 `examples/` 目录：

- **[tool_calling](./examples/tool_calling)** - 完整的工具调用流程
- **[streaming](./examples/streaming)** - 流式输出示例

运行示例：

```bash
# 工具调用
cd examples/tool_calling
export OPENAI_API_KEY="..."
go run main.go -provider openai -model gpt-4o

# 流式输出
cd examples/streaming
export OPENAI_API_KEY="..."
go run main.go -provider openai -prompt "Write a poem"
```

## 🛣️ Roadmap

### Supported
- ✅ MVP（核心接口 + 御三家支持）
- ✅ Tool Calling
- ✅ 流式支持
- ✅ 重试机制 + 错误处理增强

### Building
-  Prompt Caching（Anthropic）
-  成本计算与 Usage 追踪
-  Context 取消增强
-  工具并发执行
-  More LLM Provider Support
-  基础工具支持




## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情

---

**Star ⭐️ 本项目以获取更新通知！**
