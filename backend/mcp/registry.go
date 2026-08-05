package mcp

import (
	"encoding/json"
	"fmt"
	"os"

	"alice/config"
)

// RegistryItem MCP 市场中的一个可安装项
type RegistryItem struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Transport   string            `json:"transport,omitempty"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	Env         []string          `json:"env,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// Registry MCP 注册表（市场数据源，本地文件或远程 URL）
type Registry struct {
	Version string         `json:"version"`
	Servers []RegistryItem `json:"servers"`
}

// LoadRegistry 从本地文件或远程 URL 加载注册表
func LoadRegistry(source, path string) (*Registry, error) {
	var data []byte
	var err error
	if source == "remote" {
		data, err = fetchURL(path)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("加载注册表失败: %w", err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("解析注册表失败: %w", err)
	}
	return &reg, nil
}

// Item 按 ID 查找注册表项
func (r *Registry) Item(id string) (RegistryItem, bool) {
	for _, s := range r.Servers {
		if s.ID == id {
			return s, true
		}
	}
	return RegistryItem{}, false
}

// ToConfig 注册表项 → MCP 配置
func (it RegistryItem) ToConfig() config.MCPServerConfig {
	return config.MCPServerConfig{
		ID:        it.ID,
		Name:      it.Name,
		Transport: it.Transport,
		Command:   it.Command,
		Args:      it.Args,
		Env:       it.Env,
		URL:       it.URL,
		Headers:   it.Headers,
		Enabled:   true,
	}
}
