package main

import (
	"log"
	"myvpn/pkg/utils" // Proje ismine göre burayı kontrol et
	"net"
	"sync/atomic"

	"github.com/songgao/water"
)

// DİKKAT: Anahtar tam 32 karakter olmalı. Client ile aynı olmalı!
var vpnKey = []byte("12345678901234567890123456789012")

func main() {
	// TUN cihazını oluşturuyoruz
	tun := utils.CreateTUN("10.0.0.1", "10.0.0.2", "utun6")

	// Sunucuyu başlatıyoruz
	socketServer(tun)

	select {}
}

func socketServer(tun *water.Interface) {
	// --- PORT 8080 (TCP Taşıyıcı) Kurulumu ---
	addrTCP, _ := net.ResolveUDPAddr("udp", ":8080")
	connTCP, err := net.ListenUDP("udp", addrTCP)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Listening for encrypted TCP-embedded packets on :8080")

	// --- PORT 8081 (UDP/ICMP Taşıyıcı) Kurulumu ---
	addrUDP, _ := net.ResolveUDPAddr("udp", ":8081")
	connUDP, err := net.ListenUDP("udp", addrUDP)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Listening for encrypted UDP-embedded packets on :8081")

	// İstemci adreslerini saklamak için
	var clientAddrForTCP atomic.Value
	var clientAddrForUDP atomic.Value

	// --- AĞDAN GELENİ OKU VE ÇÖZ (Network -> TUN) ---
	go readFromNetToTun(connTCP, tun, &clientAddrForTCP, "TCP-Line")
	go readFromNetToTun(connUDP, tun, &clientAddrForUDP, "UDP-Line")

	// --- TUN'DAN GELENİ ŞİFRELE VE GÖNDER (TUN -> Network) ---
	buf := make([]byte, 2000)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			log.Println("TUN Read Error:", err)
			continue
		}

		// 1. Önce paketin içine bakıyoruz (TCP mi UDP mi?)
		packet := utils.ParseIPv4(buf[:n])

		// 2. TÜM PAKETİ ŞİFRELİYORUZ (AES-GCM)
		// Gönderdiğimiz şey artık orijinal paket değil, bir kara kutu!
		encrypted, err := utils.Encrypt(buf[:n], vpnKey)
		if err != nil {
			log.Println("Şifreleme hatası:", err)
			continue
		}

		if packet.Protocol == utils.ProtocolTCP {
			addr := clientAddrForTCP.Load()
			if addr != nil {
				// Şifreli veriyi (encrypted) gönderiyoruz
				connTCP.WriteToUDP(encrypted, addr.(*net.UDPAddr))
				log.Printf("[TUN->Net] Encrypted packet sent via Port 8080")
			}
		} else {
			addr := clientAddrForUDP.Load()
			if addr != nil {
				// Şifreli veriyi (encrypted) gönderiyoruz
				connUDP.WriteToUDP(encrypted, addr.(*net.UDPAddr))
				log.Printf("[TUN->Net] Encrypted packet sent via Port 8081")
			}
		}
	}
}

// readFromNetToTun: İnternetten gelen kara kutuları (şifreli paketleri) açar.
func readFromNetToTun(conn *net.UDPConn, tun *water.Interface, remoteAddr *atomic.Value, label string) {
	buf := make([]byte, 2000)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		// İstemci adresini hafızaya alıyoruz
		remoteAddr.Store(clientAddr)

		// --- ŞİFREYİ ÇÖZME ADIMI ---
		// Gelen 'buf[:n]' bir kara kutu. Onu anahtarla açıyoruz.
		decrypted, err := utils.Decrypt(buf[:n], vpnKey)
		if err != nil {
			// Eğer birisi anahtarı bilmiyorsa veya paket bozulmuşsa buraya düşer.
			log.Printf("[%s] GÜVENLİK UYARISI: Geçersiz şifreli paket reddedildi!", label)
			continue
		}

		// 3. Çözülen orijinal paketi işletim sistemine (TUN) veriyoruz
		tun.Write(decrypted)
		log.Printf("[Net->TUN] %s üzerinden gelen paket başarıyla çözüldü", label)
	}
}
