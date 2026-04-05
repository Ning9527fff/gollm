package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"goLLM/llm"
)

// ProviderConfig Provider 配置
type ProviderConfig struct {
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
}

// ConfigFile 配置文件结构
type ConfigFile struct {
	Providers map[string]ProviderConfig `json:"providers"`
}

// 默认配置文件路径
const DefaultConfigPath = "./llm-config.json"

// LoadConfig 加载指定 provider 的配置
// 优先级：环境变量 > JSON 文件 > 默认值
func LoadConfig(provider string) (*llm.Config, error) {
	// 1. 尝试从环境变量加载
	cfg, err := LoadFromEnv(provider)
	if err == nil && cfg.APIKey != "" {
		return cfg, nil
	}

	// 2. 尝试从默认配置文件加载
	configs, err := LoadFromFile(DefaultConfigPath)
	if err == nil {
		if cfg, ok := configs[provider]; ok {
			return cfg, nil
		}
	}

	// 3. 返回错误（无可用配置）
	return nil, fmt.Errorf("config: no configuration found for provider %s", provider)
}

// LoadFromFile 从 JSON 文件加载所有配置
func LoadFromFile(path string) (map[string]*llm.Config, error) {
	// 读取文件
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read file: %w", err)
	}

	// 解析 JSON
	var configFile ConfigFile
	if err := json.Unmarshal(data, &configFile); err != nil {
		return nil, fmt.Errorf("config: parse json: %w", err)
	}

	// 转换为 llm.Config
	result := make(map[string]*llm.Config)
	for provider, cfg := range configFile.Providers {
		result[provider] = &llm.Config{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
		}
	}

	return result, nil
}

// LoadFromEnv 从环境变量加载指定 provider 配置
// 环境变量命名规则：
//   - {PROVIDER}_API_KEY (必需)
//   - {PROVIDER}_BASE_URL (可选)
//   - {PROVIDER}_DEFAULT_MODEL (可选)
func LoadFromEnv(provider string) (*llm.Config, error) {
	prefix := strings.ToUpper(provider)

	apiKey := os.Getenv(fmt.Sprintf("%s_API_KEY", prefix))
	if apiKey == "" {
		return nil, fmt.Errorf("config: %s_API_KEY not set", prefix)
	}

	return &llm.Config{
		APIKey:  apiKey,
		BaseURL: os.Getenv(fmt.Sprintf("%s_BASE_URL", prefix)),
	}, nil
}

// LoadConfigWithFallback 加载配置，支持指定配置文件路径
// 优先级：环境变量 > 指定文件 > 默认文件
func LoadConfigWithFallback(provider string, configPath string) (*llm.Config, error) {
	// 1. 尝试从环境变量加载
	cfg, err := LoadFromEnv(provider)
	if err == nil && cfg.APIKey != "" {
		return cfg, nil
	}

	// 2. 尝试从指定配置文件加载
	if configPath != "" {
		configs, err := LoadFromFile(configPath)
		if err == nil {
			if cfg, ok := configs[provider]; ok {
				return cfg, nil
			}
		}
	}

	// 3. 尝试从默认配置文件加载
	configs, err := LoadFromFile(DefaultConfigPath)
	if err == nil {
		if cfg, ok := configs[provider]; ok {
			return cfg, nil
		}
	}

	return nil, fmt.Errorf("config: no configuration found for provider %s", provider)
}
