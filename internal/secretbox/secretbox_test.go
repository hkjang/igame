package secretbox

import "testing"

func TestRoundTripAndRandomNonce(t *testing.T) {
	b, err := New([]byte("12345678901234567890123456789012"))
	if err != nil {
		t.Fatal(err)
	}
	one, _ := b.Seal("secret")
	two, _ := b.Seal("secret")
	if one == two {
		t.Fatal("ciphertexts should differ")
	}
	got, err := b.Open(one)
	if err != nil || got != "secret" {
		t.Fatalf("got %q, %v", got, err)
	}
}
