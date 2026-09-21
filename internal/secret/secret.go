// Package secret handles encrypted application-managed secrets.
// Model: AES-256-GCM with a master key from SCOUT_MASTER_KEY env or a
// 0600 key file in the data dir. Secrets are never logged, never returned
// by normal APIs, never committed. Tradeoff: anyone with the master key +
// DB can decrypt; documented in SECURITY.md.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func MasterKey(dataDir string, fromEnv string) ([]byte, error) {
	if fromEnv != "" {
		return normalizeKey([]byte(fromEnv)), nil
	}
	kf := filepath.Join(dataDir, ".masterkey")
	if b, err := os.ReadFile(kf); err == nil && len(b) >= 16 {
		return normalizeKey(b), nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(kf, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func normalizeKey(b []byte) []byte {
	out := make([]byte, 32)
	copy(out, b)
	return out
}

func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(normalizeKey(key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func Decrypt(key, blob []byte) ([]byte, error) {
	block, err := aes.NewCipher(normalizeKey(key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return pt, nil
}
