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

	// BaseDir 配置文件目录（绝对路径），用于解析相对路径（如 MCP command）
	BaseDir string
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
	// Audio 语音能力（阶段：TTS/STT）
	Audio struct {
		TTSEnabled  bool   `yaml:"tts_enabled"`  // 回复完成后自动合成语音并推送 assistant_audio
		TTSVoice    string `yaml:"tts_voice"`    // 音色（缺省用 TTS Server 默认）
		STTEnabled  bool   `yaml:"stt_enabled"`  // 语音输入转文字
		MaxUploadMB int    `yaml:"max_upload_mb"` // 文件分块上传大小上限
	} `yaml:"audio"`
	SystemPrompt string `yaml:"-"`
}

type EmotionEvent struct {
	Keywords []string          `yaml:"keywords"`
	Changes  map[string]float64 `yaml:"changes"`
	Desc     string             `yaml:"desc"` // 事件简短中文描述（注入上下文用）
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
	// 关系矩阵：目标维度 ← 来源维度: 系数（正=促进，负=抑制），tick 时演化（漂移/拮抗）
	Relations  map[string]map[string]float64 `yaml:"relations"`
	Proactive  struct {
		Enabled          bool    `yaml:"enabled"`
		Threshold        float64 `yaml:"threshold"`
		CooldownSec      int     `yaml:"cooldown_seconds"`
		TickSec          int     `yaml:"tick_seconds"`
		SilentAfterMin   int     `yaml:"silent_after_minutes"` // 用户长时间无互动触发主动关心，0=关闭
		SkipIfActiveMin  int     `yaml:"skip_if_active_minutes"` // 用户最近互动过则跳过主动推送，0=不跳过
		Hours            []int   `yaml:"hours"`                // 允许主动推送的小时段 [起, 止]，空=不限
	} `yaml:"proactive"`
	Persistence struct {
		Enabled bool `yaml:"enabled"`
		TTLSec  int  `yaml:"ttl_seconds"`
	} `yaml:"persistence"`
	EventMap       map[string]EmotionEvent `yaml:"-"`
	Templates      []EmotionTemplate       `yaml:"-"`
	ProactivePrompt string                 `yaml:"-"` // 主动推送提示词模板（{emotion} 占位）
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

// MCPServerConfig 已安装的 MCP Server 定义
type MCPServerConfig struct {
	ID        string            `yaml:"id"`
	Name      string            `yaml:"name"`
	Transport string            `yaml:"transport"` // "stdio"（默认）/ "http"
	Command   string            `yaml:"command"`   // stdio：启动命令（可执行文件或 npx）
	Args      []string          `yaml:"args"`
	Env       []string          `yaml:"env"` // "KEY=VALUE"
	URL       string            `yaml:"url"` // http：MCP 端点地址
	Headers   map[string]string `yaml:"headers"`
	Enabled   bool              `yaml:"enabled"`
	// Internal 内部 Server：工具不暴露给 LLM（如 TTS/STT，音频数据不应进 LLM 上下文），
	// 但可由后端代码直接调用（serverID__toolName）
	Internal bool `yaml:"internal"`
}

type MCPConfig struct {
	Registry struct {
		Source string `yaml:"source"`
		File   string `yaml:"file"`
	} `yaml:"registry"`
	Servers   []MCPServerConfig `yaml:"servers"`
	AutoStart bool              `yaml:"auto_start"`
	TimeoutSec int              `yaml:"timeout_seconds"` // MCP 启动超时（秒）
}

// Load 从 configDir 加载全部 YAML 配置
func Load(configDir string) (*Config, error) {
	cfg := &Config{}
	absDir, err := filepath.Abs(configDir)
	if err != nil {
		return nil, err
	}
	cfg.BaseDir = absDir

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

	// 主动推送提示词模板
	proactivePrompt, err := os.ReadFile(filepath.Join(configDir, "prompts", "proactive_prompt.txt"))
	if err != nil {
		cfg.Emotion.ProactivePrompt = "你现在想主动联系用户说句话。{emotion} 请直接给出一句自然、真诚的关心或问候，一句话即可，不要解释。"
	} else {
		cfg.Emotion.ProactivePrompt = string(proactivePrompt)
	}

	// System Prompt（阶段一即启用）
	promptBytes, err := os.ReadFile(filepath.Join(configDir, "prompts", "system_prompt.txt"))
	if err != nil {
		return nil, fmt.Errorf("读取 system_prompt.txt 失败: %w", err)
	}
	cfg.Kernel.SystemPrompt = string(promptBytes)

	// API Key 支持环境变量覆盖（配置文件保持干净，密钥不进仓库）
	if cfg.Kernel.LLM.APIKey == "" {
		cfg.Kernel.LLM.APIKey = os.Getenv("ALICE_LLM_API_KEY")
	}
	if cfg.RAG.Embedding.APIKey == "" {
		cfg.RAG.Embedding.APIKey = os.Getenv("ALICE_EMBED_API_KEY")
	}

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

// UpdateMCP 更新 mcp.yaml 的 servers 列表并写回（安装/卸载用）。
// 注意：会重写文件（不保留注释），随后 config.Watch 热重载自动生效。
func UpdateMCP(configDir string, servers []MCPServerConfig) error {
	path := filepath.Join(configDir, "mcp.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	mcpNode, ok := root["mcp"].(map[string]any)
	if !ok {
		return fmt.Errorf("mcp.yaml 缺少顶层 mcp 段")
	}
	mcpNode["servers"] = servers

	out, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// UpdateYAMLValue 修改 YAML 文件指定路径的值并写回（路径中间键自动创建）。
// 用 yaml.Node 操作：保留注释与键顺序，随后 config.Watch 热重载自动生效。
func UpdateYAMLValue(configDir, filename string, path []string, value any) error {
	if len(path) == 0 {
		return fmt.Errorf("路径不能为空")
	}
	filePath := filepath.Join(configDir, filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	if len(root.Content) == 0 {
		root.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	cur := root.Content[0]

	for i, key := range path {
		last := i == len(path)-1
		if cur.Kind != yaml.MappingNode {
			return fmt.Errorf("路径 %v 处不是映射，无法写入", path[:i])
		}
		var found *yaml.Node
		for j := 0; j+1 < len(cur.Content); j += 2 {
			if cur.Content[j].Value == key {
				found = cur.Content[j+1]
				break
			}
		}
		if found == nil {
			found = &yaml.Node{}
			cur.Content = append(cur.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, found)
		}
		if last {
			repl, err := toYAMLNode(value)
			if err != nil {
				return err
			}
			*found = *repl
		} else if found.Kind != yaml.MappingNode {
			*found = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		cur = found
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, out, 0o644)
}

// toYAMLNode 把任意值序列化为 yaml.Node（保留类型：标量/序列/映射）
func toYAMLNode(v any) (*yaml.Node, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	var n yaml.Node
	if err := yaml.Unmarshal(data, &n); err != nil {
		return nil, err
	}
	if len(n.Content) > 0 {
		return n.Content[0], nil
	}
	return &n, nil
}

// llmSettingPaths 设置键 → kernel.yaml 路径（文件顶层无命名空间）
var llmSettingPaths = map[string][]string{
	"provider":    {"llm", "provider"},
	"base_url":    {"llm", "base_url"},
	"api_key":     {"llm", "api_key"},
	"model":       {"llm", "model"},
	"temperature": {"llm", "temperature"},
	"max_tokens":  {"llm", "max_tokens"},
}

// emotionSettingPaths 设置键 → emotion.yaml 路径（文件有顶层 emotion 命名空间）
var emotionSettingPaths = map[string][]string{
	"decay_rate":             {"emotion", "decay_rate"},
	"max_value":              {"emotion", "max_value"},
	"threshold":              {"emotion", "proactive", "threshold"},
	"cooldown_seconds":       {"emotion", "proactive", "cooldown_seconds"},
	"tick_seconds":           {"emotion", "proactive", "tick_seconds"},
	"silent_after_minutes":   {"emotion", "proactive", "silent_after_minutes"},
	"skip_if_active_minutes": {"emotion", "proactive", "skip_if_active_minutes"},
	"hours":                  {"emotion", "proactive", "hours"},
}

// ApplySettings 把运行时设置写回 YAML（随后 watcher 热重载自动生效）。
// section: "llm" / "emotion" / "block"；values 键见各 SettingPaths，未知键忽略。
func ApplySettings(configDir, section string, values map[string]any) error {
	var paths map[string][]string
	var filename string
	switch section {
	case "llm":
		filename, paths = "kernel.yaml", llmSettingPaths
	case "emotion":
		filename, paths = "emotion.yaml", emotionSettingPaths
	case "block":
		filename = "memory_block.yaml"
		paths = map[string][]string{"max_entries": {"memory_block", "max_entries"}}
	default:
		return fmt.Errorf("未知设置段: %s", section)
	}
	for key, value := range values {
		path, ok := paths[key]
		if !ok {
			continue // 忽略未知键
		}
		if err := UpdateYAMLValue(configDir, filename, path, value); err != nil {
			return fmt.Errorf("写入 %s.%s 失败: %w", section, key, err)
		}
	}
	return nil
}
