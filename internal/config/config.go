package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Machines   map[string]Machine `yaml:"machines"`
	Services   map[string]Service `yaml:"services"`
	Remotes    map[string]Remote  `yaml:"remotes,omitempty"`
	API        APIConfig          `yaml:"api,omitempty"`
	Cloudflare CloudflareConfig   `yaml:"cloudflare"`
}

type Remote struct {
	URL string `yaml:"url"`
	Key string `yaml:"key,omitempty"`
}

type APIConfig struct {
	Key      string `yaml:"key,omitempty"`
	ReadOnly bool   `yaml:"read_only,omitempty"`
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

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var validUser = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
var validIP = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$`)
var validHostname = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]+$`)

func ValidateName(name string) error {
	if len(name) == 0 || len(name) > 64 {
		return fmt.Errorf("nom invalide (1-64 caractères)")
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("nom invalide: uniquement lettres, chiffres, points, tirets, underscores")
	}
	return nil
}

func ValidateUser(user string) error {
	if len(user) == 0 || len(user) > 32 {
		return fmt.Errorf("utilisateur invalide (1-32 caractères)")
	}
	if !validUser.MatchString(user) {
		return fmt.Errorf("utilisateur invalide: uniquement lettres, chiffres, points, tirets, underscores")
	}
	return nil
}

func ValidateIP(ip string) error {
	if !validIP.MatchString(ip) {
		return fmt.Errorf("IP invalide: format attendu x.x.x.x")
	}
	return nil
}

func ValidateURL(url string) error {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("URL invalide: doit commencer par http:// ou https://")
	}
	// Block private/link-local for SSRF prevention on remote URLs
	lower := strings.ToLower(url)
	blockedPrefixes := []string{
		"http://169.254.", "https://169.254.",
		"http://127.", "https://127.",
		"http://0.", "https://0.",
		"http://localhost", "https://localhost",
		"http://[::1]", "https://[::1]",
	}
	for _, prefix := range blockedPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return fmt.Errorf("URL invalide: adresses locales/link-local non autorisées pour les remotes")
		}
	}
	return nil
}

func ValidateTunnel(hostname string) error {
	if hostname == "" {
		return nil
	}
	if !validHostname.MatchString(hostname) {
		return fmt.Errorf("hostname tunnel invalide")
	}
	return nil
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

func GenerateAPIKey() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
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
	path := ConfigPath()
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

func Init() error {
	dir := HopDir()
	dirs := []string{dir, filepath.Join(dir, "dotfiles")}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
	}

	// Create .gitignore to prevent syncing secrets
	gitignorePath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		gitignore := "# Ne pas sync les secrets\napi.key\n"
		os.WriteFile(gitignorePath, []byte(gitignore), 0600)
	}

	if _, err := os.Stat(ConfigPath()); os.IsNotExist(err) {
		cfg := &Config{
			Machines: make(map[string]Machine),
			Services: map[string]Service{
				"ssh":      {Desc: "Connexion SSH", Builtin: true},
				"rustdesk": {Desc: "Connexion Rustdesk", Builtin: true},
			},
		}
		return cfg.Save()
	}
	return nil
}
