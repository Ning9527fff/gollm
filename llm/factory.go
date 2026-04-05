package llm

import "fmt"

// ProviderFactory 创建 LLM 实例的工厂函数
type ProviderFactory func(config Config) (LLM, error)

// 注册表
var registry = make(map[string]ProviderFactory)

// Register 注册 LLM provider
func Register(name string, factory ProviderFactory) {
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("provider %s already registered", name))
	}
	registry[name] = factory
}

// NewLLM 创建 LLM 实例
func NewLLM(provider string, config Config) (LLM, error) {
	factory, ok := registry[provider]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
	return factory(config)
}

// ListProviders 列出所有已注册的 provider
func ListProviders() []string {
	providers := make([]string, 0, len(registry))
	for name := range registry {
		providers = append(providers, name)
	}
	return providers
}
