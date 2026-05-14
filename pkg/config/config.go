package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	DefaultServerIP = "192.168.64.6"
	DefaultPort     = 8079
	DefaultVPNKey   = "12345678901234567890123456789012"
	DefaultOutIface = "eth0"
)

// ClientConfig istemci tarafı ayarlarını tutar.
type ClientConfig struct {
	ServerIP string `json:"server_ip"`
	Port     int    `json:"port"`
	VPNKey   string `json:"vpn_key"`
}

// ServerConfig sunucu tarafı ayarlarını tutar.
type ServerConfig struct {
	Port     int    `json:"port"`
	VPNKey   string `json:"vpn_key"`
	OutIface string `json:"out_iface"`
}

func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".myvpn")
	return dir, os.MkdirAll(dir, 0700)
}

// LoadClientConfig istemci ayarlarını ~/.myvpn/client.json dosyasından yükler.
// Dosya yoksa veya bozuksa varsayılan değerleri döndürür.
func LoadClientConfig() *ClientConfig {
	dir, err := configDir()
	if err != nil {
		return defaultClientConfig()
	}
	data, err := os.ReadFile(filepath.Join(dir, "client.json"))
	if err != nil {
		return defaultClientConfig()
	}
	cfg := &ClientConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return defaultClientConfig()
	}
	return cfg
}

// Save istemci ayarlarını ~/.myvpn/client.json dosyasına kaydeder.
func (c *ClientConfig) Save() error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "client.json"), data, 0600)
}

func defaultClientConfig() *ClientConfig {
	return &ClientConfig{
		ServerIP: DefaultServerIP,
		Port:     DefaultPort,
		VPNKey:   DefaultVPNKey,
	}
}

// LoadServerConfig sunucu ayarlarını ~/.myvpn/server.json dosyasından yükler.
func LoadServerConfig() *ServerConfig {
	dir, err := configDir()
	if err != nil {
		return defaultServerConfig()
	}
	data, err := os.ReadFile(filepath.Join(dir, "server.json"))
	if err != nil {
		return defaultServerConfig()
	}
	cfg := &ServerConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return defaultServerConfig()
	}
	return cfg
}

// Save sunucu ayarlarını ~/.myvpn/server.json dosyasına kaydeder.
func (c *ServerConfig) Save() error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "server.json"), data, 0600)
}

func defaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:     DefaultPort,
		VPNKey:   DefaultVPNKey,
		OutIface: DefaultOutIface,
	}
}
