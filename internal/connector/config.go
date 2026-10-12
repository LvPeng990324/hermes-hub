package connector

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 连接器配置（YAML 文件）
type Config struct {
	PlatformURL      string `yaml:"platform_url"`      // 平台地址，如 http://192.168.1.10:8080
	Token            string `yaml:"token"`             // 实例接入令牌（平台注册实例时生成）
	TimeoutSeconds   int    `yaml:"timeout_seconds"`   // 单次指令执行超时
	ReconnectSeconds int    `yaml:"reconnect_seconds"` // 断线重连间隔

	ACP ACPConfig `yaml:"acp"` // Hermes ACP 子进程配置
}

// LoadConfig 读取并校验连接器配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	cfg := &Config{
		TimeoutSeconds:   1800,
		ReconnectSeconds: 5,
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	if cfg.PlatformURL == "" {
		return nil, fmt.Errorf("配置缺少 platform_url")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("配置缺少 token")
	}
	if cfg.ACP.Command == "" {
		return nil, fmt.Errorf("配置缺少 acp.command（Hermes CLI 路径）")
	}
	return cfg, nil
}
