package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Machines   map[string]Machine  `yaml:"machines"`
	Services   map[string]Service  `yaml:"services"`
	Cloudflare CloudflareConfig    `yaml:"cloudflare"`
}

type Machine struct {
	IP       string                    `yaml:"ip"`
	User     string                    `yaml:"user"`
	Tunnel   string                    `yaml:"tunnel,omitempty"`
	Services map[string]MachineService `yaml:"services,omitempty"`
}

type MachineService struct {
	ID  string `yaml:"id,omitempty"`
	Cmd string `yaml:"cmd,omitempty"`
}

type Service struct {
	Desc    string `yaml:"desc"`
	Cmd     string `yaml:"cmd,omitempty"`
	Builtin bool   `yaml:"builtin,omitempty"`
}

type CloudflareConfig struct {
	Domain  string `yaml:"domain,omitempty"`
	EnvFile string `yaml:"env_file,omitempty"`
}

func HopDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hop")
}

func ConfigPath() string {
	return filepath.Join(HopDir(), "config.yml")
}

func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		return home + path[1:]
	}
	return path
}

func Load() (*Config, error) {
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		return nil, fmt.Errorf("cannot read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse config: %w", err)
	}

	if cfg.Machines == nil {
		cfg.Machines = make(map[string]Machine)
	}
	if cfg.Services == nil {
		cfg.Services = make(map[string]Service)
	}

	return &cfg, nil
}

func (c *Config) Save() error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("cannot marshal config: %w", err)
	}
	return os.WriteFile(ConfigPath(), data, 0644)
}

func Init() error {
	dir := HopDir()
	dirs := []string{dir, filepath.Join(dir, "dotfiles")}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}

	if _, err := os.Stat(ConfigPath()); os.IsNotExist(err) {
		cfg := &Config{
			Machines: make(map[string]Machine),
			Services: map[string]Service{
				"ssh": {Desc: "Connexion SSH", Builtin: true},
				"rustdesk": {Desc: "Connexion Rustdesk", Builtin: true},
			},
		}
		return cfg.Save()
	}
	return nil
}
