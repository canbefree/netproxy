package config

import (
	"encoding/json"
	"os"
)

// Config 全局配置
type Config struct {
	Server ServerConfig `json:"server"`
	Client ClientConfig `json:"client"`
}

// ServerConfig 服务端配置
type ServerConfig struct {
	ListenAddr string `json:"listen_addr"` // 服务端监听地址
	Token      string `json:"token"`       // 认证令牌
}

// ClientConfig 客户端配置
type ClientConfig struct {
	ServerAddr string          `json:"server_addr"` // 服务端地址
	Token      string          `json:"token"`       // 认证令牌
	Proxies    []ProxyConfig   `json:"proxies"`     // 代理规则
}

// ProxyConfig 代理规则配置
type ProxyConfig struct {
	Name       string `json:"name"`        // 代理名称
	Type       string `json:"type"`        // 代理类型 (tcp)
	LocalAddr  string `json:"local_addr"`  // 本地地址
	RemotePort int    `json:"remote_port"` // 远程端口
}

// LoadConfig 从文件加载配置
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveConfig 保存配置到文件
func SaveConfig(config *Config, filePath string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}
