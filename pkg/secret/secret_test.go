package secret

import "testing"

func TestEncryptRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	ct, err := Encrypt(key, []byte("oauth-token-123"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := Decrypt(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "oauth-token-123" {
		t.Fatal("roundtrip mismatch")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	k1 := make([]byte, 32)
	k2 := make([]byte, 32)
	k2[0] = 1
	ct, _ := Encrypt(k1, []byte("x"))
	if _, err := Decrypt(k2, ct); err == nil {
		t.Fatal("expected failure with wrong key")
	}
}
