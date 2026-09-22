package config

import (
	"strings"
	"testing"
)

func TestParseEncryptionKey(t *testing.T) {
	for _, in := range []string{
		"12345678901234567890123456789012",
		"hex:3132333435363738393031323334353637383930313233343536373839303132",
		"base64:MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=",
	} {
		got, err := ParseEncryptionKey(in)
		if err != nil || len(got) != 32 {
			t.Fatalf("ParseEncryptionKey(%q) = %d, %v", in, len(got), err)
		}
	}
	if _, err := ParseEncryptionKey("short"); err == nil {
		t.Fatal("expected short key error")
	}
}

func TestLoadRejectsWeakBootstrapPassword(t *testing.T) {
	t.Setenv(EnvPostgresDSN, "postgres://example/igame")
	t.Setenv(EnvBootstrapAdmin, "admin")
	t.Setenv(EnvBootstrapAdminPass, "too-short")
	t.Setenv(EnvEncryptionKey, "12345678901234567890123456789012")
	if _, err := Load(); err == nil {
		t.Fatal("expected a weak bootstrap password error")
	}

	t.Setenv(EnvBootstrapAdminPass, "long-enough-12")
	if _, err := Load(); err != nil {
		t.Fatalf("expected a valid bootstrap password, got %v", err)
	}
}

func TestLoadBootstrapPasswordBoundaries(t *testing.T) {
	t.Setenv(EnvPostgresDSN, "postgres://example/igame")
	t.Setenv(EnvBootstrapAdmin, "admin")
	t.Setenv(EnvEncryptionKey, "12345678901234567890123456789012")
	for _, tc := range []struct {
		name      string
		password  string
		wantError string
	}{
		{"missing", "", "missing required environment variables"},
		{"ascii11", strings.Repeat("x", 11), "at least 12 characters"},
		{"korean11", strings.Repeat("가", 11), "at least 12 characters"},
		{"ascii12", strings.Repeat("x", 12), ""},
		{"ascii72", strings.Repeat("x", 72), ""},
		{"ascii73", strings.Repeat("x", 73), "at most 72 bytes"},
		{"korean12", strings.Repeat("가", 12), ""},
		{"korean24", strings.Repeat("가", 24), ""},
		{"korean25", strings.Repeat("가", 25), "at most 72 bytes"},
		{"mixed72", strings.Repeat("가", 23) + "abc", ""},
		{"mixed73", strings.Repeat("가", 23) + "abcd", "at most 72 bytes"},
		{"spaces_preserved", " " + strings.Repeat("x", 70) + " ", ""},
		{"spaces_count_toward_limit", " " + strings.Repeat("x", 71) + " ", "at most 72 bytes"},
		{"decomposed_unicode_preserved", strings.Repeat("e\u0301", 12), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvBootstrapAdminPass, tc.password)
			got, err := Load()
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("Load failed: %v", err)
				}
				if got.BootstrapPassword != tc.password {
					t.Fatal("Load changed the bootstrap password")
				}
				return
			}
			if err == nil {
				t.Fatal("Load accepted an invalid bootstrap password")
			}
			if !strings.Contains(err.Error(), EnvBootstrapAdminPass) || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.password != "" && strings.Contains(err.Error(), tc.password) {
				t.Fatal("error exposed the bootstrap password")
			}
		})
	}
}
