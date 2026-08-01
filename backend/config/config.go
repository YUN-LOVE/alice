package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config 全量配置，由各模块配置聚合而成
type Config struct {
	Main    *MainConfig
	Kernel  *KernelConfig
	Emotion *EmotionConfig
	RAG     *RAGConfig
	Block   *BlockConfig
	MCP     *MCPConfig
}

type MainConfig struct {
	Server struct {
		Host   string `yaml:"host"`
		Port   int    `yaml:"port"`
		WSPath string `yaml:"ws_path"`
	} `yaml:"server"`
	LogLevel string `yaml:"log_level"`
}

type KernelConfig struct {
	LLM struct {
		Provider    string  `yaml:"provider"`
		BaseURL     string  `yaml:"base_url"`
		APIKey      string  `yaml:"api_key"`
		Model       string  `yaml:"model"`
		Temperature float64 `yaml:"temperature"`
		MaxTokens   int     `yaml:"max_tokens"`
		Stream      bool    `yaml:"stream"`
	} `yaml:"llm"`
	Context struct {
		MaxMessages int `yaml:"max_messages"`
	} `yaml:"context"`
	SystemPrompt string `yaml:"-"`
}

type EmotionEvent struct {
	Keywords []string          `yaml:"keywords"`
	Changes  map[string]float64 `yaml:"changes"`
}

type EmotionTemplate struct {
	Name       string                 `yaml:"name"`
	Conditions map[string][2]float64  `yaml:"conditions"`
	Text       string                 `yaml:"text"`
	Proactive  string                 `yaml:"proactive"`
}

type EmotionConfig struct {
	Dimensions []string          `yaml:"dimensions"`
	Initial    map[string]float64 `yaml:"initial"`
	Baseline   map[string]float64 `yaml:"baseline"`
	DecayRate  float64           `yaml:"decay_rate"`
	MaxValue   float64           `yaml:"max_value"`
	Proactive  struct {
		Enabled       bool    `yaml:"enabled"`
		Threshold     float64 `yaml:"threshold"`
		CooldownSec   int     `yaml:"cooldown_seconds"`
		TickSec       int     `yaml:"tick_seconds"`
	} `yaml:"proactive"`
	Persistence struct {
		Enabled bool `yaml:"enabled"`
		TTLSec  int  `yaml:"ttl_seconds"`
	} `yaml:"persistence"`
	EventMap  map[string]EmotionEvent `yaml:"-"`
	Templates []EmotionTemplate       `yaml:"-"`
}

type RAGConfig struct {
	Embedding struct {
		Provider string `yaml:"provider"`
		BaseURL  string `yaml:"base_url"`
		APIKey   string `yaml:"api_key"`
		Model    string `yaml:"model"`
	} `yaml:"embedding"`
	Redis struct {
		Addr     string `yaml:"addr"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
	} `yaml:"redis"`
	Retrieval struct {
		TopK     int     `yaml:"top_k"`
		MinScore float64 `yaml:"min_score"`
	} `yaml:"retrieval"`
	StoreEveryTurn bool `yaml:"store_every_turn"`
}

type BlockConfig struct {
	MaxEntries int  `yaml:"max_entries"`
	ReadOnly   bool `yaml:"read_only"`
}

type MCPConfig struct {
	Registry struct {
		Source string `yaml:"source"`
		File   string `yaml:"file"`
	} `yaml:"registry"`
	Servers   []map[string]interface{} `yaml:"servers"`
	AutoStart bool                     `yaml:"auto_start"`
}

// Load 从 configDir 加载全部 YAML 配置
func Load(configDir string) (*Config, error) {
	cfg := &Config{}

	var err error
	if cfg.Main, err = loadNamed[MainConfig](configDir, "main.yaml", ""); err != nil {
		return nil, err
	}
	if cfg.Kernel, err = loadNamed[KernelConfig](configDir, "kernel.yaml", ""); err != nil {
		return nil, err
	}
	if cfg.Emotion, err = loadNamed[EmotionConfig](configDir, "emotion.yaml", "emotion"); err != nil {
		return nil, err
	}
	if cfg.RAG, err = loadNamed[RAGConfig](configDir, "memory_rag.yaml", "rag"); err != nil {
		return nil, err
	}
	if cfg.Block, err = loadNamed[BlockConfig](configDir, "memory_block.yaml", "memory_block"); err != nil {
		return nil, err
	}
	if cfg.MCP, err = loadNamed[MCPConfig](configDir, "mcp.yaml", "mcp"); err != nil {
		return nil, err
	}

	// 情绪事件映射与情绪模板（独立文件）
	eventMap, err := loadNamed[map[string]EmotionEvent](configDir, "emotion_events.yaml", "events")
	if err != nil {
		return nil, err
	}
	cfg.Emotion.EventMap = *eventMap

	tpls, err := loadNamed[[]EmotionTemplate](configDir, "prompts/emotion_templates.yaml", "templates")
	if err != nil {
		return nil, err
	}
	cfg.Emotion.Templates = *tpls

	// System Prompt（阶段一即启用）
	promptBytes, err := os.ReadFile(filepath.Join(configDir, "prompts", "system_prompt.txt"))
	if err != nil {
		return nil, fmt.Errorf("读取 system_prompt.txt 失败: %w", err)
	}
	cfg.Kernel.SystemPrompt = string(promptBytes)

	return cfg, nil
}

// loadNamed 加载 YAML 文件；key 非空时先从顶层命名空间解包
func loadNamed[T any](configDir, filename, key string) (*T, error) {
	path := filepath.Join(configDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	if key == "" {
		var v T
		if err := yaml.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
		}
		return &v, nil
	}
	var wrapped map[string]T
	if err := yaml.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	v, ok := wrapped[key]
	if !ok {
		return nil, fmt.Errorf("解析 %s 失败: 缺少顶层键 %q", path, key)
	}
	return &v, nil
}
