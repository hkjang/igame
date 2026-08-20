package config

import "testing"

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
