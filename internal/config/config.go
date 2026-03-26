package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Machines   map[string]Machine `yaml:"machines"`
	Services   map[string]Service `yaml:"services"`
	Aliases    map[string]string  `yaml:"aliases,omitempty"`
	Remotes    map[string]Remote  `yaml:"remotes,omitempty"`
	API        APIConfig          `yaml:"api,omitempty"`
	Cloudflare CloudflareConfig   `yaml:"cloudflare"`
	WorkerURL  string             `yaml:"worker_url,omitempty"`
}

type Remote struct {
	URL string `yaml:"url"`
	Key string `yaml:"-"` // Never serialize keys to config.yml; they belong in secrets.yml
}

type APIConfig struct {
	Key      string `yaml:"-"`
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
	Tmux    bool   `yaml:"tmux,omitempty"`
	Session string `yaml:"session,omitempty"`
	NoPerm  bool   `yaml:"noperm,omitempty"`
}

type CloudflareConfig struct {
	Domain  string `yaml:"domain,omitempty"`
	EnvFile string `yaml:"env_file,omitempty"`
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

func ValidateURL(rawURL string) error {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return fmt.Errorf("URL invalide: doit commencer par http:// ou https://")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL invalide")
	}

	// Resolve hostname and check if it points to a private/loopback IP
	host := parsed.Hostname()
	ips, err := net.LookupHost(host)
	if err != nil {
		// Can't resolve — allow (might be a tunnel hostname not yet set up)
		return nil
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
			return fmt.Errorf("URL invalide: pointe vers une adresse locale/privee")
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

func ValidateRustdeskID(id string) error {
	if id == "" {
		return nil
	}
	if !validRustdeskID.MatchString(id) {
		return fmt.Errorf("ID Rustdesk invalide: uniquement alphanumerique")
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

func SecretsPath() string {
	return filepath.Join(HopDir(), "secrets.yml")
}

func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		path = home + path[1:]
	}
	cleaned := filepath.Clean(path)
	// Ensure it stays within home directory
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(cleaned, home) {
		return filepath.Join(home, filepath.Base(cleaned))
	}
	return cleaned
}

func GenerateAPIKey() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Secrets stored separately from config (gitignored)
type Secrets struct {
	APIKey     string            `yaml:"api_key,omitempty"`
	RemoteKeys map[string]string `yaml:"remote_keys,omitempty"`
}

func LoadSecrets() (*Secrets, error) {
	data, err := os.ReadFile(SecretsPath())
	if err != nil {
		return &Secrets{RemoteKeys: make(map[string]string)}, nil
	}
	var s Secrets
	if err := yaml.Unmarshal(data, &s); err != nil {
		return &Secrets{RemoteKeys: make(map[string]string)}, nil
	}
	if s.RemoteKeys == nil {
		s.RemoteKeys = make(map[string]string)
	}
	return &s, nil
}

func (s *Secrets) Save() error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	path := SecretsPath()
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
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
	dirs := []string{dir, filepath.Join(dir, "dotfiles")}
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
				"ssh":      {Desc: "Connexion SSH", Builtin: true},
				"rustdesk": {Desc: "Connexion Rustdesk", Builtin: true},
			},
		}
		return cfg.Save()
	}
	return nil
}
