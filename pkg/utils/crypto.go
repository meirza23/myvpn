package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// Encrypt: Veriyi AES-GCM ile zırhlar.
// Dönüş: [12 byte Nonce] + [Şifreli Veri + 16 byte Auth Tag]
func Encrypt(plaintext []byte, key []byte) ([]byte, error) {
	// AES makinesini 32 byte'lık anahtarla kuruyoruz
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// GCM (Galois/Counter Mode) mühürleme modunu açıyoruz
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Her paket için benzersiz (rastgele) 12 byte numara üretiyoruz
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Seal: Veriyi şifreler ve nonce'u paketin en başına yapıştırır
	// Format: [Nonce][Ciphertext+Tag]
	return aesGCM.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt: Şifreli paketi açar ve doğruluğunu (mührünü) kontrol eder.
func Decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("paket çok kısa")
	}

	// Paketi parçala: İlk 12 byte nonce, kalanı şifreli veri
	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Open: Şifreyi çözer. Eğer mühür bozulmuşsa hata döner.
	return aesGCM.Open(nil, nonce, actualCiphertext, nil)
}
