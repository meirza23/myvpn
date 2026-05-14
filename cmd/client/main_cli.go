package main

import (
	"bufio"
	"fmt"
	"log"
	"myvpn/pkg/config"
	"myvpn/pkg/utils"
	"myvpn/pkg/vpn"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/songgao/water"
)

// Bağlantı durumu
var (
	connected bool
	serverIP  string
	origGW    string
	origDev   string
	tunIface  *water.Interface
	stopChan  chan struct{}
	vpnKey    []byte
)

func main() {
	// Config yükle
	cfg := config.LoadClientConfig()
	vpnKey = []byte(cfg.VPNKey)

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
	utils.PrintInfo(fmt.Sprintf("Ayarlar yüklendi — Sunucu: %s, Port: %d", cfg.ServerIP, cfg.Port))

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
				connect(scanner, cfg)
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

func connect(scanner *bufio.Scanner, cfg *config.ClientConfig) {
	// Kullanıcıdan IP iste (boş bırakılırsa config değeri kullanılır)
	fmt.Printf("\n  Sunucu IP [%s]: ", cfg.ServerIP)
	if !scanner.Scan() {
		return
	}
	input := strings.TrimSpace(scanner.Text())
	if input != "" {
		serverIP = input
	} else {
		serverIP = cfg.ServerIP
	}

	if len(vpnKey) != 32 {
		utils.PrintError(fmt.Sprintf("VPN anahtarı 32 karakter olmalı (şu an: %d). ~/.myvpn/client.json dosyasını kontrol edin.", len(vpnKey)))
		return
	}

	// 1. Handshake
	utils.PrintInfo("Sunucuya bağlanılıyor (handshake)...")
	serverAddr := fmt.Sprintf("%s:%d", serverIP, cfg.Port)
	assignedIP, err := performHandshake(serverAddr)
	if err != nil {
		utils.PrintError("Handshake başarısız: " + err.Error())
		return
	}
	utils.PrintSuccess(fmt.Sprintf("Sanal IP atandı: %s", assignedIP))

	// 2. TUN oluştur
	utils.PrintInfo("TUN arayüzü oluşturuluyor...")
	tun, err := utils.CreateTUN(assignedIP.String(), "10.0.0.1", "")
	if err != nil {
		utils.PrintError("TUN hatası: " + err.Error())
		return
	}
	tunIface = tun
	utils.PrintInfo("TUN: " + tun.Name())

	// 3. Routing
	utils.PrintInfo("Routing kuralları uygulanıyor...")
	origGW, origDev = utils.SetupClientRouting(serverIP, tun.Name())

	// 4. Tüneli başlat
	stopChan = make(chan struct{})
	go startVPNTunnel(tun, serverIP)

	connected = true
	utils.PrintSuccess(fmt.Sprintf("VPN bağlantısı kuruldu! (%s → %s)", assignedIP, serverIP))
}

func disconnect() {
	if !connected {
		return
	}
	utils.PrintInfo("VPN bağlantısı kesiliyor...")

	if stopChan != nil {
		close(stopChan)
	}
	utils.CleanupClientRouting(serverIP, origGW, origDev)

	connected = false
	serverIP = ""
	tunIface = nil
	utils.PrintSuccess("VPN bağlantısı kesildi. Orijinal routing geri yüklendi.")
}

// performHandshake sunucuya Hello gönderir ve atanan sanal IP'yi alır.
func performHandshake(serverAddr string) (net.IP, error) {
	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := conn.Write(vpn.HelloPacket); err != nil {
		return nil, fmt.Errorf("hello gönderilemedi: %w", err)
	}

	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("welcome beklerken zaman aşımı (sunucu çalışıyor mu?): %w", err)
	}

	return vpn.ParseWelcomePacket(buf[:n])
}

func startVPNTunnel(tun *water.Interface, srvIP string) {
	// TCP taşıyıcı: port 8080
	serverAddrTCP, _ := net.ResolveUDPAddr("udp", srvIP+":8080")
	connTCP, err := net.DialUDP("udp", nil, serverAddrTCP)
	if err != nil {
		utils.PrintError("TCP bağlantı hatası: " + err.Error())
		return
	}

	// UDP taşıyıcı: port 8081
	serverAddrUDP, _ := net.ResolveUDPAddr("udp", srvIP+":8081")
	connUDP, err := net.DialUDP("udp", nil, serverAddrUDP)
	if err != nil {
		utils.PrintError("UDP bağlantı hatası: " + err.Error())
		return
	}

	log.Println("Veri kanalları açıldı: :8080 (TCP-bearer), :8081 (UDP-bearer)")

	go readFromNetToTun(connTCP, tun, stopChan, "TCP-bearer")
	go readFromNetToTun(connUDP, tun, stopChan, "UDP-bearer")

	buf := make([]byte, 65535)
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

		packet := utils.ParseIPv4(buf[:n])
		encrypted, err := utils.Encrypt(buf[:n], vpnKey)
		if err != nil {
			log.Println("Şifreleme hatası:", err)
			continue
		}

		if packet.Protocol == utils.ProtocolTCP {
			connTCP.Write(encrypted)
		} else {
			connUDP.Write(encrypted)
		}
	}
}

func readFromNetToTun(conn *net.UDPConn, tun *water.Interface, stop chan struct{}, label string) {
	buf := make([]byte, 65535)
	for {
		select {
		case <-stop:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			// Timeout ise döngüyü sürdür (stop channel kontrolü için)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// Gerçek hata (bağlantı kapatıldı vb.) — gor outine'i sonlandır
			return
		}
		decrypted, err := utils.Decrypt(buf[:n], vpnKey)
		if err != nil {
			log.Printf("[%s] Şifre çözme hatası: paket reddedildi", label)
			continue
		}
		tun.Write(decrypted)
	}
}
