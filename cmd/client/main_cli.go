package main

import (
	"bufio"
	"fmt"
	"log"
	"myvpn/pkg/utils"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/songgao/water"
)

// DİKKAT: Anahtar tam 32 karakter olmalı. Server ile aynı olmalı!
var vpnKey = []byte("12345678901234567890123456789012")

// Bağlantı durumu
var (
	connected bool
	serverIP  string
	origGW    string
	origDev   string
	tunIface  *water.Interface
	stopChan  chan struct{}
)

func main() {
	// CTRL+C yakalama
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println()
		disconnect()
		os.Exit(0)
	}()

	utils.PrintBanner()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		utils.PrintStatus(connected, serverIP)
		utils.PrintMenu(connected)

		if !scanner.Scan() {
			break
		}
		choice := strings.TrimSpace(scanner.Text())

		switch choice {
		case "1":
			if !connected {
				connect(scanner)
			} else {
				disconnect()
			}
		case "2":
			utils.PrintStatus(connected, serverIP)
		case "3":
			disconnect()
			utils.PrintInfo("Güle güle!")
			return
		default:
			utils.PrintError("Geçersiz seçim. Lütfen 1, 2 veya 3 girin.")
		}
	}
}

func connect(scanner *bufio.Scanner) {
	fmt.Print("\n  Sunucu IP Adresi (ör. 192.168.64.6): ")
	if !scanner.Scan() {
		return
	}
	serverIP = strings.TrimSpace(scanner.Text())
	if serverIP == "" {
		utils.PrintError("IP adresi boş olamaz.")
		return
	}

	utils.PrintInfo("TUN arayüzü oluşturuluyor...")

	// TUN oluştur (Client: 10.0.0.2, Peer: 10.0.0.1)
	tunIface = utils.CreateTUN("10.0.0.2", "10.0.0.1", "utun5")

	// Routing ayarla
	utils.PrintInfo("Routing kuralları uygulanıyor...")
	origGW, origDev = utils.SetupClientRouting(serverIP, tunIface.Name())

	// VPN tünelini başlat
	stopChan = make(chan struct{})
	go startVPNTunnel(tunIface, serverIP)

	connected = true
	utils.PrintSuccess(fmt.Sprintf("VPN bağlantısı kuruldu! (%s)", serverIP))
}

func disconnect() {
	if !connected {
		return
	}

	utils.PrintInfo("VPN bağlantısı kesiliyor...")

	// Stop sinyali gönder
	if stopChan != nil {
		close(stopChan)
	}

	// Routing'i temizle
	utils.CleanupClientRouting(serverIP, origGW, origDev)

	connected = false
	serverIP = ""
	tunIface = nil
	utils.PrintSuccess("VPN bağlantısı kesildi. Orijinal routing geri yüklendi.")
}

func startVPNTunnel(tun *water.Interface, srvIP string) {
	// 1. Bağlantı: TCP paketlerini taşıyacak (Hedef Port 8080)
	serverAddrTCP, _ := net.ResolveUDPAddr("udp", srvIP+":8080")
	connTCP, err := net.DialUDP("udp", nil, serverAddrTCP)
	if err != nil {
		utils.PrintError("TCP bağlantı hatası: " + err.Error())
		return
	}

	// 2. Bağlantı: UDP paketlerini taşıyacak (Hedef Port 8081)
	serverAddrUDP, _ := net.ResolveUDPAddr("udp", srvIP+":8081")
	connUDP, err := net.DialUDP("udp", nil, serverAddrUDP)
	if err != nil {
		utils.PrintError("UDP bağlantı hatası: " + err.Error())
		return
	}

	log.Println("Connected to server ports 8080 (TCP-Line) and 8081 (UDP-Line) with AES-256-GCM")

	// --- SERVER'DAN GELEN CEVAPLARI DİNLE VE ÇÖZ (Net -> TUN) ---
	go readFromNetToTun(connTCP, tun, "TCP-Line")
	go readFromNetToTun(connUDP, tun, "UDP-Line")

	// --- TUN'DAN OKU, ŞİFRELE VE GÖNDER (TUN -> Net) ---
	buf := make([]byte, 2000)
	for {
		select {
		case <-stopChan:
			connTCP.Close()
			connUDP.Close()
			return
		default:
		}

		n, err := tun.Read(buf)
		if err != nil {
			continue
		}

		// 1. Paketin protokolüne bak
		packet := utils.ParseIPv4(buf[:n])

		// 2. TÜM PAKETİ ŞİFRELE (AES-GCM)
		encrypted, err := utils.Encrypt(buf[:n], vpnKey)
		if err != nil {
			log.Println("Şifreleme hatası:", err)
			continue
		}

		// 3. Şifrelenmiş veriyi uygun porttan gönder
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

		// Şifreyi çöz
		decrypted, err := utils.Decrypt(buf[:n], vpnKey)
		if err != nil {
			log.Printf("[%s] Şifre çözme hatası: Paket reddedildi", label)
			continue
		}

		// Çözülen paketi TUN'a yaz
		tun.Write(decrypted)
		log.Printf("[Net->TUN] Received and decrypted via %s", label)
	}
}
