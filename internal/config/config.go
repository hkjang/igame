// Package config loads igame's deliberately small bootstrap configuration.
// All mutable service configuration is stored in PostgreSQL.
package config

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	EnvPostgresDSN        = "POSTGRES_DSN"
	EnvBootstrapAdmin     = "BOOTSTRAP_ADMIN"
	EnvBootstrapAdminPass = "BOOTSTRAP_ADMIN_PASSWORD"
	EnvEncryptionKey      = "ENCRYPTION_KEY"
)

type Config struct {
	PostgresDSN       string
	BootstrapAdmin    string
	BootstrapPassword string
	EncryptionKey     []byte
}

func Load() (Config, error) {
	c := Config{
		PostgresDSN:       strings.TrimSpace(os.Getenv(EnvPostgresDSN)),
		BootstrapAdmin:    strings.TrimSpace(os.Getenv(EnvBootstrapAdmin)),
		BootstrapPassword: os.Getenv(EnvBootstrapAdminPass),
	}
	var missing []string
	if c.PostgresDSN == "" {
		missing = append(missing, EnvPostgresDSN)
	}
	if c.BootstrapAdmin == "" {
		missing = append(missing, EnvBootstrapAdmin)
	}
	if c.BootstrapPassword == "" {
		missing = append(missing, EnvBootstrapAdminPass)
	}
	keyText := strings.TrimSpace(os.Getenv(EnvEncryptionKey))
	if keyText == "" {
		missing = append(missing, EnvEncryptionKey)
	}
	if len(missing) != 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	if utf8.RuneCountInString(c.BootstrapPassword) < 12 {
		return Config{}, fmt.Errorf("%s must be at least 12 characters", EnvBootstrapAdminPass)
	}
	key, err := ParseEncryptionKey(keyText)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", EnvEncryptionKey, err)
	}
	c.EncryptionKey = key
	return c, nil
}

func ParseEncryptionKey(value string) ([]byte, error) {
	if strings.HasPrefix(value, "base64:") {
		b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "base64:"))
		if err != nil {
			return nil, errors.New("invalid base64 value")
		}
		return validateKey(b)
	}
	if strings.HasPrefix(value, "hex:") {
		b, err := hex.DecodeString(strings.TrimPrefix(value, "hex:"))
		if err != nil {
			return nil, errors.New("invalid hex value")
		}
		return validateKey(b)
	}
	// A plain 32-byte value keeps first-time offline installation simple.
	return validateKey([]byte(value))
}

func validateKey(key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("must decode to exactly 32 bytes, got %d", len(key))
	}
	return append([]byte(nil), key...), nil
}
