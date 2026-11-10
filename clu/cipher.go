package clu

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
)

// Errors of the cipher (spec §8, CryptoError).
var (
	// ErrInvalidKeyMaterial: the key is not 16, 24 or 32 bytes, or the IV
	// not 16, or either is not base64.
	ErrInvalidKeyMaterial = errors.New("invalid key material")
	// ErrEncryptFailed is never returned by the standard library cipher;
	// it exists for other Cipher implementations.
	ErrEncryptFailed = errors.New("encryption failed")
)

// Cipher is the frame cipher of spec §5.1, §7.2: AES-CBC with PKCS#7 and the
// interface's static IV. It is obfuscation, not security: the protocol has
// no integrity or authentication, and identical frames encrypt identically.
type Cipher interface {
	Encrypt(plaintext []byte) ([]byte, error)
	// Decrypt returns false for a ciphertext that is not a whole number of
	// blocks or whose padding is invalid; it never panics.
	Decrypt(ciphertext []byte) ([]byte, bool)
}

// AESCipher is the production [Cipher]. It is safe for concurrent use.
type AESCipher struct {
	block cipher.Block
	iv    []byte
}

// NewCipher builds the cipher from the interface's base64 key and IV.
func NewCipher(keyBase64, ivBase64 string) (*AESCipher, error) {
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, ErrInvalidKeyMaterial
	}
	iv, err := base64.StdEncoding.DecodeString(ivBase64)
	if err != nil || len(iv) != aes.BlockSize {
		return nil, ErrInvalidKeyMaterial
	}
	switch len(key) {
	case 16, 24, 32:
	default:
		return nil, ErrInvalidKeyMaterial
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKeyMaterial
	}
	return &AESCipher{block: block, iv: iv}, nil
}

// Encrypt pads with PKCS#7 and encrypts in CBC mode.
func (c *AESCipher) Encrypt(plaintext []byte) ([]byte, error) {
	pad := aes.BlockSize - len(plaintext)%aes.BlockSize
	buf := make([]byte, len(plaintext)+pad)
	copy(buf, plaintext)
	for i := len(plaintext); i < len(buf); i++ {
		buf[i] = byte(pad)
	}
	cipher.NewCBCEncrypter(c.block, c.iv).CryptBlocks(buf, buf)
	return buf, nil
}

// Decrypt decrypts and strictly checks the padding: 1–16 bytes, all equal
// to the length.
func (c *AESCipher) Decrypt(ciphertext []byte) ([]byte, bool) {
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, false
	}
	buf := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(c.block, c.iv).CryptBlocks(buf, ciphertext)
	pad := int(buf[len(buf)-1])
	if pad < 1 || pad > aes.BlockSize {
		return nil, false
	}
	for _, b := range buf[len(buf)-pad:] {
		if int(b) != pad {
			return nil, false
		}
	}
	return buf[:len(buf)-pad], true
}
