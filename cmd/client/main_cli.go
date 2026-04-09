package main

import (
	"flag"
	"log"
	"myvpn/pkg/utils" // go.mod'daki proje ismine göre kontrol et
	"net"

	"github.com/songgao/water"
)

// DİKKAT: Anahtar tam 32 karakter olmalı. Server ile aynı olmalı!
var vpnKey = []byte("12345678901234567890123456789012")

func main() {
	serverIP := flag.String("server", "", "Server IP Address (e.g. 192.168.64.6)")
	flag.Parse()
	if *serverIP == "" {
		log.Fatal("Usage: go run main.go -server 192.168.64.6")
	}

	// Client TUN IP: 10.0.0.2, Peer (Server): 10.0.0.1
	tun := utils.CreateTUN("10.0.0.2", "10.0.0.1", "utun5")

	startClient(tun, *serverIP)
}

func startClient(tun *water.Interface, serverIP string) {
	// 1. Bağlantı: TCP paketlerini taşıyacak (Hedef Port 8080)
	serverAddrTCP, _ := net.ResolveUDPAddr("udp", serverIP+":8080")
	connTCP, err := net.DialUDP("udp", nil, serverAddrTCP)
	if err != nil {
		log.Fatal(err)
	}

	// 2. Bağlantı: UDP paketlerini taşıyacak (Hedef Port 8081)
	serverAddrUDP, _ := net.ResolveUDPAddr("udp", serverIP+":8081")
	connUDP, err := net.DialUDP("udp", nil, serverAddrUDP)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Connected to server ports 8080 (TCP-Line) and 8081 (UDP-Line) with AES-256-GCM")

	// --- SERVER'DAN GELEN CEVAPLARI DİNLE VE ÇÖZ (Net -> TUN) ---
	go readFromNetToTun(connTCP, tun, "TCP-Line")
	go readFromNetToTun(connUDP, tun, "UDP-Line")

	// --- TUN'DAN OKU, ŞİFRELE VE GÖNDER (TUN -> Net) ---
	buf := make([]byte, 2000)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			continue
		}

		// 1. Önce paketin içine bakıyoruz (Hangi yoldan gideceğine karar vermek için)
		packet := utils.ParseIPv4(buf[:n])

		// 2. TÜM PAKETİ ŞİFRELİYORUZ (AES-GCM)
		// Orijinal IP paketini bir kara kutuya çeviriyoruz.
		encrypted, err := utils.Encrypt(buf[:n], vpnKey)
		if err != nil {
			log.Println("Şifreleme hatası:", err)
			continue
		}

		// 3. Şifrelenmiş veriyi (encrypted) uygun porttan gönderiyoruz
		if packet.Protocol == utils.ProtocolTCP {
			connTCP.Write(encrypted)
			log.Printf("[TUN->Net] Encrypted TCP routed to 8080")
		} else {
			connUDP.Write(encrypted)
			log.Printf("[TUN->Net] Encrypted UDP/ICMP routed to 8081")
		}
	}
}

// readFromNetToTun: Sunucudan gelen şifreli paketleri çözer.
func readFromNetToTun(conn *net.UDPConn, tun *water.Interface, label string) {
	buf := make([]byte, 2000)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		// --- ŞİFREYİ ÇÖZME ADIMI ---
		// Sunucudan gelen 'buf[:n]' bir kara kutu. Onu anahtarla açıyoruz.
		decrypted, err := utils.Decrypt(buf[:n], vpnKey)
		if err != nil {
			// Anahtar yanlışsa veya paket internette bozulmuşsa hata verir.
			log.Printf("[%s] Şifre çözme hatası: Paket reddedildi", label)
			continue
		}

		// 4. Çözülen orijinal paketi TUN üzerinden bilgisayarımıza veriyoruz
		tun.Write(decrypted)
		log.Printf("[Net->TUN] Received and decrypted via %s", label)
	}
}
