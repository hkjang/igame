package config

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

type encryptionKeyCase struct {
	name    string
	input   string
	payload string
	want    []byte // nil means the input must be rejected.
}

func encryptionKeyCases() []encryptionKeyCase {
	// Synthetic, non-repeating bytes make incorrect decoding visible.
	const material = "0123456789abcdefghijklmnopqrstuvw"
	var cases []encryptionKeyCase
	for _, format := range []struct {
		name   string
		encode func([]byte) string
	}{
		{"plain", func(b []byte) string { return string(b) }},
		{"hex", func(b []byte) string { return "hex:" + hex.EncodeToString(b) }},
		{"base64", func(b []byte) string { return "base64:" + base64.StdEncoding.EncodeToString(b) }},
	} {
		for _, size := range []int{0, 16, 24, 31, 32, 33} {
			payload := material[:size]
			var want []byte
			if size == 32 {
				want = []byte(payload)
			}
			cases = append(cases, encryptionKeyCase{
				name:  fmt.Sprintf("%s/%d_bytes", format.name, size),
				input: format.encode([]byte(payload)), payload: payload, want: want,
			})
		}
	}
	hexKey := hex.EncodeToString([]byte(material[:32]))
	base64Key := base64.StdEncoding.EncodeToString([]byte(material[:32]))
	utf8Key := strings.Repeat("가", 10) + "ab"
	return append(cases,
		encryptionKeyCase{name: "short", input: "short", payload: "short"},
		encryptionKeyCase{name: "hex/odd_length", input: "hex:" + hexKey[:63], payload: hexKey[:63]},
		encryptionKeyCase{name: "hex/non_hex", input: "hex:z" + hexKey[1:], payload: "z" + hexKey[1:]},
		encryptionKeyCase{name: "base64/invalid_character", input: "base64:!" + base64Key[1:], payload: "!" + base64Key[1:]},
		encryptionKeyCase{name: "base64/missing_padding", input: "base64:" + strings.TrimRight(base64Key, "="), payload: strings.TrimRight(base64Key, "=")},
		encryptionKeyCase{name: "plain/utf8_32_bytes", input: utf8Key, payload: utf8Key, want: []byte(utf8Key)},
		encryptionKeyCase{name: "plain/utf8_32_characters", input: strings.Repeat("가", 32), payload: strings.Repeat("가", 32)},
	)
}

func assertEncryptionKeyResult(t *testing.T, tc encryptionKeyCase, got []byte, err error) {
	t.Helper()
	if tc.want != nil {
		if err != nil {
			t.Fatal("valid encryption key was rejected")
		}
		if !bytes.Equal(got, tc.want) {
			t.Fatal("decoded encryption key does not match the expected bytes")
		}
		return
	}
	if err == nil {
		t.Fatal("invalid encryption key was accepted")
	}
	if len(got) != 0 {
		t.Error("rejected input returned an encryption key")
	}
	encodedPayload := strings.TrimPrefix(strings.TrimPrefix(tc.input, "hex:"), "base64:")
	for _, payload := range []string{tc.payload, encodedPayload} {
		if payload != "" && strings.Contains(err.Error(), payload) {
			t.Error("error exposed the encryption key payload")
		}
	}
}

func TestParseEncryptionKey(t *testing.T) {
	for _, tc := range encryptionKeyCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseEncryptionKey(tc.input)
			assertEncryptionKeyResult(t, tc, got, err)
		})
	}
}

func TestLoadEncryptionKeyContract(t *testing.T) {
	t.Setenv(EnvPostgresDSN, "postgres://example/igame")
	t.Setenv(EnvBootstrapAdmin, "admin")
	t.Setenv(EnvBootstrapAdminPass, "long-enough-12")
	cases := encryptionKeyCases()
	// Only Load trims outer whitespace; the parser receives the input as-is.
	for _, tc := range encryptionKeyCases() {
		if tc.want != nil {
			tc.name += "/outer_whitespace"
			tc.input = " \t\n" + tc.input + "\n\t "
			cases = append(cases, tc)
		}
	}
	cases = append(cases, encryptionKeyCase{name: "whitespace_only", input: " \t\n "})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvEncryptionKey, tc.input)
			got, err := Load()
			assertEncryptionKeyResult(t, tc, got.EncryptionKey, err)
			if err != nil && !strings.Contains(err.Error(), EnvEncryptionKey) {
				t.Error("Load failed for a setting other than the encryption key")
			}
		})
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
