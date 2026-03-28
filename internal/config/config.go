package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Machines   map[string]Machine `yaml:"machines"`
	Services   map[string]Service `yaml:"services"`
	Aliases    map[string]string  `yaml:"aliases,omitempty"`
	Cloudflare CloudflareConfig   `yaml:"cloudflare"`
	WorkerURL  string             `yaml:"worker_url,omitempty"`
	AIEnabled   bool   `yaml:"ai_enabled,omitempty"`
	MCPEndpoint string `yaml:"mcp_endpoint,omitempty"`
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
	Tmux    bool   `yaml:"tmux,omitempty"`
	Session string `yaml:"session,omitempty"`
	NoPerm  bool   `yaml:"noperm,omitempty"`
}

type CloudflareConfig struct {
	Domain             string `yaml:"domain,omitempty"`
	EnvFile            string `yaml:"env_file,omitempty"`
	CFServiceTokenID   string `yaml:"cf_service_token_id,omitempty"`
	CFServiceTokenSecret string `yaml:"cf_service_token_secret,omitempty"`
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var validUser = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
var validHostname = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]+$`)
var validRustdeskID = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

func ValidateName(name string) error {
	if len(name) == 0 || len(name) > 64 {
		return fmt.Errorf("nom invalide (1-64 caracteres)")
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("nom invalide: uniquement lettres, chiffres, points, tirets, underscores")
	}
	return nil
}

func ValidateUser(user string) error {
	if len(user) == 0 || len(user) > 32 {
		return fmt.Errorf("utilisateur invalide (1-32 caracteres)")
	}
	if !validUser.MatchString(user) {
		return fmt.Errorf("utilisateur invalide")
	}
	return nil
}

func ValidateIP(ip string) error {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return fmt.Errorf("IP invalide")
	}
	// Block special addresses for machine targets
	if parsed.IsLoopback() || parsed.IsUnspecified() || parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() {
		return fmt.Errorf("IP invalide: adresse speciale non autorisee")
	}
	return nil
}

func ValidateTunnel(hostname string) error {
	if hostname == "" {
		return nil
	}
	// Accept host:port format (Pinggy quick tunnels)
	host := hostname
	if idx := strings.LastIndex(hostname, ":"); idx > 0 {
		host = hostname[:idx]
	}
	if !validHostname.MatchString(host) {
		return fmt.Errorf("hostname tunnel invalide")
	}
	return nil
}

func ValidateRustdeskID(id string) error {
	if id == "" {
		return nil
	}
	if !validRustdeskID.MatchString(id) {
		return fmt.Errorf("ID Rustdesk invalide: uniquement alphanumerique")
	}
	return nil
}

// IsInstalled returns true if hop is in installed mode (~/.hop exists with installed marker)
func IsInstalled() bool {
	home, _ := os.UserHomeDir()
	_, err := os.Stat(filepath.Join(home, ".hop", ".installed"))
	return err == nil
}

// HopDir returns the config directory
// Windows: always %LOCALAPPDATA%\hop
// Sandbox mode (default): /tmp/hop-<uid>/
// Installed mode: ~/.hop/
func HopDir() string {
	// On Windows, always use %LOCALAPPDATA%\hop (no sandbox/uid concept)
	if runtime.GOOS == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			home, _ := os.UserHomeDir()
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "hop")
	}

	home, _ := os.UserHomeDir()
	installedDir := filepath.Join(home, ".hop")

	// If installed mode, use ~/.hop/
	if _, err := os.Stat(filepath.Join(installedDir, ".installed")); err == nil {
		return installedDir
	}

	// Sandbox mode: /tmp/hop-<uid>/
	return filepath.Join(os.TempDir(), fmt.Sprintf("hop-%d", os.Getuid()))
}

// PermanentDir returns ~/.hop/ regardless of mode (for hop install)
func PermanentDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hop")
}

func ConfigPath() string {
	return filepath.Join(HopDir(), "config.yml")
}

func ExpandPath(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		path = home
	} else if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	cleaned := filepath.Clean(path)
	// Ensure it stays within home directory
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(cleaned, home) {
		return filepath.Join(home, filepath.Base(cleaned))
	}
	return cleaned
}


// ResolveAlias returns the real name if an alias exists, otherwise the input
func (c *Config) ResolveAlias(name string) string {
	if c.Aliases != nil {
		if target, ok := c.Aliases[name]; ok {
			return target
		}
	}
	return name
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
	dirs := []string{dir}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
	}

	// Gitignore secrets
	gitignorePath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		gitignore := "# Ne pas sync les secrets\nsecrets.yml\n*.key\n*.pem\n*.env\nid_rsa*\nbin/\n"
		os.WriteFile(gitignorePath, []byte(gitignore), 0600)
	}

	if _, err := os.Stat(ConfigPath()); os.IsNotExist(err) {
		cfg := &Config{
			Machines: make(map[string]Machine),
			Services: map[string]Service{
				"ssh": {Desc: "Connexion SSH", Builtin: true},
			},
		}
		return cfg.Save()
	}
	return nil
}
