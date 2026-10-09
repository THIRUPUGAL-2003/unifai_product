package logstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const licenseTokenPrefix = "GWLIC1."

// licenseFileKey opens the .lic token. It is not the vendor signing key.
// The signed payload stays inside the token so the file does not show seats or client name.
func licenseFileKey() []byte {
	partA := []byte{
		0x91, 0x3c, 0xe7, 0x4a, 0x18, 0xb2, 0x5d, 0x06,
		0xcf, 0x77, 0x21, 0x9e, 0x44, 0xd8, 0x0b, 0x63,
	}
	partB := []byte{
		0xaa, 0x15, 0x6f, 0x82, 0x39, 0xc4, 0x5e, 0x10,
		0x7b, 0xe1, 0x48, 0x9a, 0x2d, 0xf6, 0x53, 0x0c,
	}
	key := make([]byte, 32)
	copy(key, partA)
	copy(key[16:], partB)
	return key
}

// SealLicenseFile wraps a signed envelope so the .lic file is a single key string.
func SealLicenseFile(inner []byte) (string, error) {
	block, err := aes.NewCipher(licenseFileKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, inner, []byte(licenseTokenPrefix))
	return licenseTokenPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func openLicenseFile(token string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(token, licenseTokenPrefix))
	if err != nil {
		return nil, fmt.Errorf("invalid license key")
	}
	block, err := aes.NewCipher(licenseFileKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(raw) < gcm.NonceSize() {
		return nil, fmt.Errorf("invalid license key")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(licenseTokenPrefix))
	if err != nil {
		return nil, fmt.Errorf("invalid license key")
	}
	return plain, nil
}
