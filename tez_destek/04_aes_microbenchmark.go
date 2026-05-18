// Şekil/Tablo için isteğe bağlı mikro-benchmark.
// Çalıştırma: go run 04_aes_microbenchmark.go
// AES-256-GCM'in saf CPU throughput'unu ölçer (ağ yok).
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"time"
)

func main() {
	key := make([]byte, 32)
	io.ReadFull(rand.Reader, key)
	block, _ := aes.NewCipher(key)
	g, _ := cipher.NewGCM(block)

	sizes := []int{64, 256, 1024, 1500, 4096}
	for _, sz := range sizes {
		pt := make([]byte, sz)
		io.ReadFull(rand.Reader, pt)
		nonce := make([]byte, g.NonceSize())

		iters := 200000
		start := time.Now()
		var ct []byte
		for i := 0; i < iters; i++ {
			io.ReadFull(rand.Reader, nonce)
			ct = g.Seal(nil, nonce, pt, nil)
		}
		_ = ct
		elapsed := time.Since(start)
		bytes := float64(sz) * float64(iters)
		mbps := (bytes * 8) / elapsed.Seconds() / 1e6
		fmt.Printf("Paket boyutu %5d B  →  %.0f Mb/s  (%d iter @ %v)\n", sz, mbps, iters, elapsed)
	}
}
