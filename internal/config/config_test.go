package config

import "testing"

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"pc1", false},
		{"my-machine", false},
		{"server.prod", false},
		{"test_01", false},
		{"A1", false},
		{"", true},
		{"-invalid", true},
		{".invalid", true},
		{"has space", true},
		{"a/b", true},
		{string(make([]byte, 65)), true}, // too long
	}

	for _, tt := range tests {
		err := ValidateName(tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

func TestValidateUser(t *testing.T) {
	tests := []struct {
		user    string
		wantErr bool
	}{
		{"root", false},
		{"freelux", false},
		{"user-01", false},
		{"user.name", false},
		{"", true},
		{"has space", true},
		{"user@host", true},
		{string(make([]byte, 33)), true},
	}

	for _, tt := range tests {
		err := ValidateUser(tt.user)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateUser(%q) error = %v, wantErr %v", tt.user, err, tt.wantErr)
		}
	}
}

func TestValidateIP(t *testing.T) {
	tests := []struct {
		ip      string
		wantErr bool
	}{
		{"192.168.0.1", false},
		{"10.0.0.1", false},
		{"8.8.8.8", false},
		{"2001:db8::1", false},
		{"", true},
		{"not-an-ip", true},
		{"127.0.0.1", true},    // loopback
		{"0.0.0.0", true},      // unspecified
		{"169.254.1.1", true},  // link-local
	}

	for _, tt := range tests {
		err := ValidateIP(tt.ip)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateIP(%q) error = %v, wantErr %v", tt.ip, err, tt.wantErr)
		}
	}
}

func TestValidateTunnel(t *testing.T) {
	tests := []struct {
		hostname string
		wantErr  bool
	}{
		{"", false},
		{"pc1.example.com", false},
		{"my-host.domain.dev", false},
		{"-invalid", true},
		{"has space.com", true},
	}

	for _, tt := range tests {
		err := ValidateTunnel(tt.hostname)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateTunnel(%q) error = %v, wantErr %v", tt.hostname, err, tt.wantErr)
		}
	}
}

func TestValidateRustdeskID(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"", false},
		{"abc123", false},
		{"ABC", false},
		{"has-dash", true},
		{"has space", true},
	}

	for _, tt := range tests {
		err := ValidateRustdeskID(tt.id)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateRustdeskID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
		}
	}
}

func TestGenerateAPIKey(t *testing.T) {
	key1 := GenerateAPIKey()
	key2 := GenerateAPIKey()

	if len(key1) != 64 { // 32 bytes hex = 64 chars
		t.Errorf("GenerateAPIKey() len = %d, want 64", len(key1))
	}
	if key1 == key2 {
		t.Error("GenerateAPIKey() generated two identical keys")
	}
}

func TestExpandPath(t *testing.T) {
	p := ExpandPath("~/.hop")
	if p == "~/.hop" {
		t.Error("ExpandPath did not expand ~")
	}
	if p == "" {
		t.Error("ExpandPath returned empty")
	}
}
