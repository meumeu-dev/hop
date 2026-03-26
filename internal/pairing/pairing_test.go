package pairing

import "testing"

func TestGenerateCode(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code := GenerateCode()
		if len(code) != 8 {
			t.Errorf("GenerateCode() len = %d, want 8", len(code))
		}
		// Must be lowercase alphanumeric
		for _, c := range code {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
				t.Errorf("GenerateCode() contains invalid char: %q", code)
			}
		}
		seen[code] = true
	}
	// With 100 draws from 36^8, all should be unique
	if len(seen) < 99 {
		t.Errorf("GenerateCode() poor randomness: only %d unique codes in 100 draws", len(seen))
	}
}

func TestEncryptDecrypt(t *testing.T) {
	code := "123456"
	plaintext := []byte("hello hop pairing")

	encrypted, err := Encrypt(plaintext, code)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	if encrypted == "" {
		t.Fatal("Encrypt() returned empty string")
	}

	decrypted, err := Decrypt(encrypted, code)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypt() = %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWrongCode(t *testing.T) {
	code := "123456"
	plaintext := []byte("secret data")

	encrypted, err := Encrypt(plaintext, code)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	_, err = Decrypt(encrypted, "654321")
	if err == nil {
		t.Error("Decrypt() with wrong code should fail")
	}
}

func TestEncryptDifferentCiphertexts(t *testing.T) {
	code := "999999"
	plaintext := []byte("same data")

	enc1, _ := Encrypt(plaintext, code)
	enc2, _ := Encrypt(plaintext, code)

	if enc1 == enc2 {
		t.Error("Encrypt() produced identical ciphertexts for same plaintext (nonce reuse)")
	}
}

func TestValidateSSHPublicKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"empty", "", true},
		{"garbage", "not a key", true},
		{"multi-line injection", "ssh-ed25519 AAAA...\nssh-ed25519 BBBB...", true},
		{"options prefix", "command=\"evil\" ssh-ed25519 AAAA...", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSSHPublicKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSSHPublicKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecryptInvalidData(t *testing.T) {
	_, err := Decrypt("not-base64!!!", "123456")
	if err == nil {
		t.Error("Decrypt() with invalid base64 should fail")
	}

	_, err = Decrypt("", "123456")
	if err == nil {
		t.Error("Decrypt() with empty data should fail")
	}
}
