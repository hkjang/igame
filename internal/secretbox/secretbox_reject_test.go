package secretbox

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"strings"
	"testing"
)

// aesKey returns a deterministic key of the requested length.
func aesKey(fill byte, n int) []byte { return bytes.Repeat([]byte{fill}, n) }

// TestOpenRejectsMalformedCiphertext pins down every input Open must refuse.
// The round trip is asserted in the same test so that turning Open into an
// unconditional error would not make this pass.
func TestOpenRejectsMalformedCiphertext(t *testing.T) {
	const plaintext = "installation secret"

	b, err := New(aesKey(0x2a, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b.Seal(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := b.Open(sealed); err != nil || got != plaintext {
		t.Fatalf("round trip: got %q, %v; want %q, nil", got, err, plaintext)
	}

	body := sealed[len("v1:"):]
	data, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("decode own ciphertext: %v", err)
	}
	nonceSize := b.aead.NonceSize()
	if len(data) <= nonceSize {
		t.Fatalf("sealed payload %d bytes, want more than the %d-byte nonce", len(data), nonceSize)
	}

	// Flip the last byte, which is part of the GCM tag rather than the
	// nonce, so the case tests tamper detection and not nonce mismatch.
	tampered := append([]byte(nil), data...)
	tampered[len(tampered)-1] ^= 1

	// Standard base64 pads with '=', which RawURLEncoding refuses. Guard the
	// assumption so the case cannot silently decode as valid base64.
	padded := base64.URLEncoding.EncodeToString(data)
	if !strings.HasSuffix(padded, "=") {
		t.Fatalf("padded fixture %q carries no padding", padded)
	}

	other, err := New(aesKey(0x5b, 32))
	if err != nil {
		t.Fatal(err)
	}
	otherKeySealed, err := other.Seal(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"prefix only", "v1:"},
		{"other version prefix", "v2:" + body},
		{"no prefix", body},
		{"not base64", "v1:not base64!!"},
		{"padded base64", "v1:" + padded},
		{"non url base64 alphabet", "v1:+" + body[1:]},
		{"shorter than the nonce", "v1:" + base64.RawURLEncoding.EncodeToString(data[:nonceSize-1])},
		{"nonce with no ciphertext", "v1:" + base64.RawURLEncoding.EncodeToString(data[:nonceSize])},
		{"single bit flipped in the tag", "v1:" + base64.RawURLEncoding.EncodeToString(tampered)},
		{"sealed with another key", otherKeySealed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := b.Open(tc.in)
			if err == nil {
				t.Fatalf("Open(%q) returned %q with no error", tc.in, got)
			}
			if got != "" {
				t.Errorf("Open(%q) returned plaintext %q alongside the error", tc.in, got)
			}
		})
	}
}

// TestSealAndOpenBindAdditionalData proves with cryptography, not source
// inspection, that both halves use exactly the AAD []byte("igame:v1").
func TestSealAndOpenBindAdditionalData(t *testing.T) {
	const plaintext = "additional data matters"
	key := aesKey(0x2a, 32)

	b, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("seal uses the AAD", func(t *testing.T) {
		sealed, err := b.Seal(plaintext)
		if err != nil {
			t.Fatal(err)
		}
		data, err := base64.RawURLEncoding.DecodeString(sealed[len("v1:"):])
		if err != nil {
			t.Fatal(err)
		}
		plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte("igame:v1"))
		if err != nil {
			t.Fatalf("independent AEAD could not open Seal output with AAD %q: %v", "igame:v1", err)
		}
		if string(plain) != plaintext {
			t.Fatalf("got %q, want %q", plain, plaintext)
		}
	})

	t.Run("open requires the AAD", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			aad  []byte
		}{
			{"no aad", nil},
			{"other version aad", []byte("igame:v2")},
		} {
			t.Run(tc.name, func(t *testing.T) {
				nonce := make([]byte, aead.NonceSize())
				data := aead.Seal(nonce, nonce, []byte(plaintext), tc.aad)
				wrapped := "v1:" + base64.RawURLEncoding.EncodeToString(data)
				if got, err := b.Open(wrapped); err == nil {
					t.Fatalf("Open accepted a ciphertext sealed with AAD %q and returned %q", tc.aad, got)
				}
			})
		}
	})
}

// TestNewAcceptsOnlyAESKeyLengths pins down the installation key length check.
func TestNewAcceptsOnlyAESKeyLengths(t *testing.T) {
	for _, n := range []int{16, 24, 32} {
		box, err := New(aesKey(0x11, n))
		if err != nil {
			t.Errorf("New with a %d-byte key: %v", n, err)
			continue
		}
		sealed, err := box.Seal("probe")
		if err != nil {
			t.Errorf("Seal with a %d-byte key: %v", n, err)
			continue
		}
		if got, err := box.Open(sealed); err != nil || got != "probe" {
			t.Errorf("round trip with a %d-byte key: got %q, %v", n, got, err)
		}
	}

	rejected := []struct {
		name string
		key  []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
		{"31 bytes", aesKey(0x11, 31)},
		{"33 bytes", aesKey(0x11, 33)},
	}
	for _, tc := range rejected {
		if box, err := New(tc.key); err == nil {
			t.Errorf("New(%s key) returned a box %p with no error", tc.name, box)
		}
	}
}
