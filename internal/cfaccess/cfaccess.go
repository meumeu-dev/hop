// Package cfaccess automates Cloudflare Access setup for hop tunnels.
// It creates an SSH Access application, a service token, and an Access policy
// so that cloudflared can authenticate without a browser challenge.
package cfaccess

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/meumeu-dev/hop/internal/config"
)

const cfAPIBase = "https://api.cloudflare.com/client/v4"

// CFEnv holds the values read from cloudflare.env.
type CFEnv struct {
	Email     string
	Domain    string
	APIKey    string
	AccountID string
}

// LoadCFEnv reads CF_USER, CF_DOMAIN, CF_API_KEY and CF_ACCOUNT_ID from the
// env file referenced by cfg (or from the default location).
func LoadCFEnv(cfg *config.Config) (*CFEnv, error) {
	envPath := cfg.Cloudflare.EnvFile
	if envPath == "" {
		envPath = config.HopDir() + "/cloudflare.env"
	}
	envPath = config.ExpandPath(envPath)

	data, err := os.ReadFile(envPath)
	if err != nil {
		return nil, fmt.Errorf("lecture cloudflare.env (%s): %w", envPath, err)
	}

	env := &CFEnv{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "CF_USER":
			env.Email = val
		case "CF_DOMAIN":
			env.Domain = val
		case "CF_API_KEY":
			env.APIKey = val
		case "CF_ACCOUNT_ID":
			env.AccountID = val
		}
	}

	var missing []string
	if env.Email == "" {
		missing = append(missing, "CF_USER")
	}
	if env.Domain == "" {
		missing = append(missing, "CF_DOMAIN")
	}
	if env.APIKey == "" {
		missing = append(missing, "CF_API_KEY")
	}
	if env.AccountID == "" {
		missing = append(missing, "CF_ACCOUNT_ID")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("cloudflare.env: champs manquants: %s", strings.Join(missing, ", "))
	}

	return env, nil
}

// cfClient is a thin wrapper around http.Client with CF credentials.
type cfClient struct {
	env    *CFEnv
	client *http.Client
}

func newClient(env *CFEnv) *cfClient {
	return &cfClient{
		env:    env,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *cfClient) do(method, url string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("X-Auth-Email", c.env.Email)
	req.Header.Set("X-Auth-Key", c.env.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// ── Access Application ────────────────────────────────────────────────────────

type accessApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Domain string `json:"domain"`
}

type listAppsResponse struct {
	Success bool        `json:"success"`
	Result  []accessApp `json:"result"`
	Errors  []cfError   `json:"errors"`
}

type createAppResponse struct {
	Success bool      `json:"success"`
	Result  accessApp `json:"result"`
	Errors  []cfError `json:"errors"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *cfClient) findApp(hostname string) (string, error) {
	url := fmt.Sprintf("%s/accounts/%s/access/apps", cfAPIBase, c.env.AccountID)
	data, _, err := c.do("GET", url, nil)
	if err != nil {
		return "", err
	}
	var resp listAppsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse apps: %w", err)
	}
	for _, app := range resp.Result {
		if app.Domain == hostname {
			return app.ID, nil
		}
	}
	return "", nil
}

func (c *cfClient) createApp(name, hostname string) (string, error) {
	url := fmt.Sprintf("%s/accounts/%s/access/apps", cfAPIBase, c.env.AccountID)
	payload := map[string]interface{}{
		"name":   name,
		"domain": hostname,
		"type":   "ssh",
		"session_duration": "24h",
	}
	data, _, err := c.do("POST", url, payload)
	if err != nil {
		return "", err
	}
	var resp createAppResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse create app: %w", err)
	}
	if !resp.Success {
		msgs := make([]string, len(resp.Errors))
		for i, e := range resp.Errors {
			msgs[i] = e.Message
		}
		return "", fmt.Errorf("CF API: %s", strings.Join(msgs, "; "))
	}
	return resp.Result.ID, nil
}

// ── Service Token ─────────────────────────────────────────────────────────────

type serviceToken struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ClientID  string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type listTokensResponse struct {
	Success bool           `json:"success"`
	Result  []serviceToken `json:"result"`
	Errors  []cfError      `json:"errors"`
}

type createTokenResponse struct {
	Success bool         `json:"success"`
	Result  serviceToken `json:"result"`
	Errors  []cfError    `json:"errors"`
}

// findServiceToken returns the client_id of an existing token with the given name, or "".
// Note: the secret cannot be retrieved after creation — we only check if the name exists.
func (c *cfClient) findServiceToken(name string) (string, error) {
	url := fmt.Sprintf("%s/accounts/%s/access/service_tokens", cfAPIBase, c.env.AccountID)
	data, _, err := c.do("GET", url, nil)
	if err != nil {
		return "", err
	}
	var resp listTokensResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse tokens: %w", err)
	}
	for _, t := range resp.Result {
		if t.Name == name {
			return t.ClientID, nil
		}
	}
	return "", nil
}

// createServiceToken creates a new service token and returns (clientID, clientSecret).
func (c *cfClient) createServiceToken(name string) (string, string, error) {
	url := fmt.Sprintf("%s/accounts/%s/access/service_tokens", cfAPIBase, c.env.AccountID)
	payload := map[string]string{"name": name}
	data, _, err := c.do("POST", url, payload)
	if err != nil {
		return "", "", err
	}
	var resp createTokenResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", "", fmt.Errorf("parse create token: %w", err)
	}
	if !resp.Success {
		msgs := make([]string, len(resp.Errors))
		for i, e := range resp.Errors {
			msgs[i] = e.Message
		}
		return "", "", fmt.Errorf("CF API: %s", strings.Join(msgs, "; "))
	}
	return resp.Result.ClientID, resp.Result.ClientSecret, nil
}

// ── Access Policy ─────────────────────────────────────────────────────────────

type policy struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type createPolicyResponse struct {
	Success bool     `json:"success"`
	Result  policy   `json:"result"`
	Errors  []cfError `json:"errors"`
}

type listPoliciesResponse struct {
	Success bool     `json:"success"`
	Result  []policy `json:"result"`
	Errors  []cfError `json:"errors"`
}

func (c *cfClient) findPolicy(appID, name string) (bool, error) {
	url := fmt.Sprintf("%s/accounts/%s/access/apps/%s/policies", cfAPIBase, c.env.AccountID, appID)
	data, _, err := c.do("GET", url, nil)
	if err != nil {
		return false, err
	}
	var resp listPoliciesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return false, fmt.Errorf("parse policies: %w", err)
	}
	for _, p := range resp.Result {
		if p.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (c *cfClient) createPolicy(appID, policyName, tokenClientID string) error {
	url := fmt.Sprintf("%s/accounts/%s/access/apps/%s/policies", cfAPIBase, c.env.AccountID, appID)
	payload := map[string]interface{}{
		"name":       policyName,
		"decision":   "non_identity",
		"include": []map[string]interface{}{
			{
				"service_token": map[string]string{
					"token_id": tokenClientID,
				},
			},
		},
	}
	data, _, err := c.do("POST", url, payload)
	if err != nil {
		return err
	}
	var resp createPolicyResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parse create policy: %w", err)
	}
	if !resp.Success {
		msgs := make([]string, len(resp.Errors))
		for i, e := range resp.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("CF API: %s", strings.Join(msgs, "; "))
	}
	return nil
}

// ── Tunnel API ────────────────────────────────────────────────────────────────

// TunnelInfo holds the result of creating or finding a Cloudflare tunnel.
type TunnelInfo struct {
	ID     string
	Secret string // base64-encoded tunnel secret (only set on creation)
}

// CreateTunnel creates a named Cloudflare tunnel via the API.
// If the tunnel already exists (409 conflict), it finds and returns the existing one.
func CreateTunnel(env *CFEnv, name string) (*TunnelInfo, error) {
	cl := newClient(env)

	// Generate a random 32-byte secret
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generation secret: %w", err)
	}
	b64Secret := base64.StdEncoding.EncodeToString(secret)

	url := fmt.Sprintf("%s/accounts/%s/cfd_tunnel", cfAPIBase, env.AccountID)
	payload := map[string]interface{}{
		"name":          name,
		"tunnel_secret": b64Secret,
	}
	data, status, err := cl.do("POST", url, payload)
	if err != nil {
		return nil, fmt.Errorf("creation tunnel: %w", err)
	}

	if status == 409 {
		// Tunnel already exists, find it
		info, err := FindTunnel(env, name)
		if err != nil {
			return nil, err
		}
		if info == nil {
			return nil, fmt.Errorf("tunnel '%s' existe deja mais introuvable dans la liste", name)
		}
		return info, nil
	}

	var resp struct {
		Success bool `json:"success"`
		Result  struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
		Errors []cfError `json:"errors"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse create tunnel: %w", err)
	}
	if !resp.Success {
		msgs := make([]string, len(resp.Errors))
		for i, e := range resp.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("CF API create tunnel: %s", strings.Join(msgs, "; "))
	}

	return &TunnelInfo{
		ID:     resp.Result.ID,
		Secret: b64Secret,
	}, nil
}

// FindTunnel looks up an existing tunnel by name.
func FindTunnel(env *CFEnv, name string) (*TunnelInfo, error) {
	cl := newClient(env)
	url := fmt.Sprintf("%s/accounts/%s/cfd_tunnel?name=%s&is_deleted=false", cfAPIBase, env.AccountID, name)
	data, _, err := cl.do("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("liste tunnels: %w", err)
	}

	var resp struct {
		Success bool `json:"success"`
		Result  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
		Errors []cfError `json:"errors"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse list tunnels: %w", err)
	}
	if !resp.Success {
		msgs := make([]string, len(resp.Errors))
		for i, e := range resp.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("CF API list tunnels: %s", strings.Join(msgs, "; "))
	}

	for _, t := range resp.Result {
		if t.Name == name {
			return &TunnelInfo{ID: t.ID}, nil
		}
	}
	return nil, nil
}

// GetZoneID retrieves the zone ID for a given domain.
func GetZoneID(env *CFEnv, domain string) (string, error) {
	cl := newClient(env)
	url := fmt.Sprintf("%s/zones?name=%s", cfAPIBase, domain)
	data, _, err := cl.do("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("recherche zone: %w", err)
	}

	var resp struct {
		Success bool `json:"success"`
		Result  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
		Errors []cfError `json:"errors"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse zones: %w", err)
	}
	if !resp.Success || len(resp.Result) == 0 {
		return "", fmt.Errorf("zone introuvable pour le domaine '%s'", domain)
	}
	return resp.Result[0].ID, nil
}

// CreateDNSRecord creates a CNAME record pointing to the tunnel.
// If the record already exists, it is skipped (no error).
func CreateDNSRecord(env *CFEnv, zoneID, hostname, tunnelID string) error {
	cl := newClient(env)

	// Check if record already exists
	checkURL := fmt.Sprintf("%s/zones/%s/dns_records?type=CNAME&name=%s", cfAPIBase, zoneID, hostname)
	data, _, err := cl.do("GET", checkURL, nil)
	if err != nil {
		return fmt.Errorf("verification DNS: %w", err)
	}

	var listResp struct {
		Success bool `json:"success"`
		Result  []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &listResp); err == nil && len(listResp.Result) > 0 {
		// Record already exists
		return nil
	}

	// Create the CNAME
	createURL := fmt.Sprintf("%s/zones/%s/dns_records", cfAPIBase, zoneID)
	payload := map[string]interface{}{
		"type":    "CNAME",
		"name":    hostname,
		"content": tunnelID + ".cfargotunnel.com",
		"proxied": true,
	}
	data, _, err = cl.do("POST", createURL, payload)
	if err != nil {
		return fmt.Errorf("creation DNS: %w", err)
	}

	var createResp struct {
		Success bool      `json:"success"`
		Errors  []cfError `json:"errors"`
	}
	if err := json.Unmarshal(data, &createResp); err != nil {
		return fmt.Errorf("parse create DNS: %w", err)
	}
	if !createResp.Success {
		msgs := make([]string, len(createResp.Errors))
		for i, e := range createResp.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("CF API create DNS: %s", strings.Join(msgs, "; "))
	}
	return nil
}

// ── Public entry point ────────────────────────────────────────────────────────

// SetupResult holds the service token credentials produced by Setup.
type SetupResult struct {
	TokenID     string
	TokenSecret string // empty when reusing an existing token
	Reused      bool   // true when the token already existed (secret not available)
}

// Setup ensures a CF Access application + service token + policy exist for the
// given tunnel hostname. It returns the service token credentials to be stored.
//
// If the Access app or policy already exists, those steps are skipped.
// If a service token with the same name already exists, it is reused (but the
// secret cannot be retrieved from the API — the caller should keep whatever was
// previously stored).
func Setup(cfg *config.Config, tunnelName string) (*SetupResult, error) {
	env, err := LoadCFEnv(cfg)
	if err != nil {
		return nil, err
	}

	hostname := tunnelName + "." + env.Domain
	tokenName := "hop-" + tunnelName
	policyName := "hop-" + tunnelName + "-policy"

	cl := newClient(env)

	// ── Step A: Access application ───────────────────────────────────────────
	fmt.Printf("  → Recherche de l'application CF Access pour %s...\n", hostname)
	appID, err := cl.findApp(hostname)
	if err != nil {
		return nil, fmt.Errorf("recherche app: %w", err)
	}
	if appID != "" {
		fmt.Printf("  → Application existante (id: %s)\n", appID)
	} else {
		fmt.Printf("  → Creation de l'application CF Access (ssh, %s)...\n", hostname)
		appID, err = cl.createApp("hop-"+tunnelName, hostname)
		if err != nil {
			return nil, fmt.Errorf("creation app: %w", err)
		}
		fmt.Printf("  → Application creee (id: %s)\n", appID)
	}

	// ── Step B: Service token ────────────────────────────────────────────────
	fmt.Printf("  → Recherche du service token '%s'...\n", tokenName)
	existingClientID, err := cl.findServiceToken(tokenName)
	if err != nil {
		return nil, fmt.Errorf("recherche token: %w", err)
	}

	result := &SetupResult{}

	if existingClientID != "" {
		fmt.Printf("  → Service token existant (client_id: %s)\n", existingClientID)
		fmt.Println("  ⚠ Le secret du token existant ne peut pas etre recupere depuis l'API.")
		fmt.Println("    Si le secret n'est plus disponible, supprimez le token dans le dashboard CF et relancez.")
		result.TokenID = existingClientID
		result.Reused = true
	} else {
		fmt.Printf("  → Creation du service token '%s'...\n", tokenName)
		clientID, clientSecret, err := cl.createServiceToken(tokenName)
		if err != nil {
			return nil, fmt.Errorf("creation token: %w", err)
		}
		fmt.Printf("  → Service token cree (client_id: %s)\n", clientID)
		result.TokenID = clientID
		result.TokenSecret = clientSecret
	}

	// ── Step C: Policy ───────────────────────────────────────────────────────
	fmt.Printf("  → Verification de la policy '%s'...\n", policyName)
	exists, err := cl.findPolicy(appID, policyName)
	if err != nil {
		return nil, fmt.Errorf("recherche policy: %w", err)
	}
	if exists {
		fmt.Println("  → Policy existante, rien a faire.")
	} else {
		fmt.Printf("  → Creation de la policy (service token autorise)...\n")
		if err := cl.createPolicy(appID, policyName, result.TokenID); err != nil {
			return nil, fmt.Errorf("creation policy: %w", err)
		}
		fmt.Println("  → Policy creee.")
	}

	return result, nil
}
