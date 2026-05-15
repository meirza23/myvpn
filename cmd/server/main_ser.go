package main

import (
	"flag"
	"fmt"
	"log"
	"myvpn/pkg/config"
	"myvpn/pkg/utils"
	"myvpn/pkg/vpn"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/songgao/water"
)

func main() {
	outIface := flag.String("iface", "", "Internet çıkış arayüzü (ör. eth0, ens33); boş bırakılırsa config'den okunur")
	flag.Parse()

	cfg := config.LoadServerConfig()
	if *outIface != "" {
		cfg.OutIface = *outIface
	}

	log.Printf("MyVPN Sunucusu başlatılıyor (port=%d, iface=%s)", cfg.Port, cfg.OutIface)

	vpnKey := []byte(cfg.VPNKey)
	if len(vpnKey) != 32 {
		log.Fatalf("VPN anahtarı tam 32 karakter olmalı (şu an: %d). ~/.myvpn/server.json dosyasını kontrol edin.", len(vpnKey))
	}

	// TUN cihazını oluştur
	tun, err := utils.CreateTUN("10.0.0.1", "10.0.0.2", "")
	if err != nil {
		log.Fatal("TUN hatası:", err)
	}

	// Routing kurallarını uygula
	utils.SetupServerRouting(tun.Name(), cfg.OutIface)

	// Session yöneticisi
	sessions := vpn.NewSessionManager()

	// Temiz kapanış
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("\nKapatılıyor...")
		utils.CleanupServerRouting(tun.Name(), cfg.OutIface)
		os.Exit(0)
	}()

	// Periyodik session temizleme
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			n := sessions.CleanupExpired()
			if n > 0 {
				log.Printf("[Session] %d zaman aşımına uğramış oturum temizlendi. Aktif: %d", n, sessions.Count())
			}
		}
	}()

	// Sunucuyu başlat
	startServer(tun, sessions, cfg, vpnKey)
}

func startServer(tun *water.Interface, sessions *vpn.SessionManager, cfg *config.ServerConfig, vpnKey []byte) {
	// Port 8079 — Kontrol / Handshake kanalı
	ctrlAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", cfg.Port))
	ctrlConn, err := net.ListenUDP("udp", ctrlAddr)
	if err != nil {
		log.Fatal("Kontrol portu açılamadı:", err)
	}
	log.Printf("Handshake dinleyici: :%d", cfg.Port)

	// Port 8080 — TCP taşıyıcı
	addr8080, _ := net.ResolveUDPAddr("udp", ":8080")
	conn8080, err := net.ListenUDP("udp", addr8080)
	if err != nil {
		log.Fatal("Port 8080 açılamadı:", err)
	}
	log.Println("Veri dinleyici: :8080 (TCP-bearer)")

	// Port 8081 — UDP taşıyıcı
	addr8081, _ := net.ResolveUDPAddr("udp", ":8081")
	conn8081, err := net.ListenUDP("udp", addr8081)
	if err != nil {
		log.Fatal("Port 8081 açılamadı:", err)
	}
	log.Println("Veri dinleyici: :8081 (UDP-bearer)")

	// Handshake dinleyicisi
	go handleHandshake(ctrlConn, sessions)

	// Ağdan TUN'a: şifreli paket al → çöz → TUN'a yaz
	go readNetToTUN(conn8080, sessions, tun, vpnKey, "TCP-bearer")
	go readNetToTUN(conn8081, sessions, tun, vpnKey, "UDP-bearer")

	// TUN'dan ağa: paketi oku → şifrele → doğru istemciye gönder
	buf := make([]byte, 65535)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			log.Println("TUN okuma hatası:", err)
			continue
		}

		packet := utils.ParseIPv4(buf[:n])
		encrypted, err := utils.Encrypt(buf[:n], vpnKey)
		if err != nil {
			log.Println("Şifreleme hatası:", err)
			continue
		}

		// Hedef sanal IP ve protokole göre istemcinin doğru bearer adresini bul
		clientAddr, ok := sessions.GetBearerAddrByVirtualIP(packet.DestAddr, packet.Protocol)
		if !ok {
			// Bilinmeyen hedef, atla
			continue
		}

		if packet.Protocol == utils.ProtocolTCP {
			conn8080.WriteToUDP(encrypted, clientAddr)
		} else {
			conn8081.WriteToUDP(encrypted, clientAddr)
		}
	}
}

// handleHandshake: istemciden gelen Hello paketine cevap olarak sanal IP atar.
func handleHandshake(conn *net.UDPConn, sessions *vpn.SessionManager) {
	buf := make([]byte, 64)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		if !vpn.IsHelloPacket(buf[:n]) {
			log.Printf("[Handshake] Geçersiz paket %s'den, atlanıyor.", clientAddr)
			continue
		}

		sess, err := sessions.Register(clientAddr)
		if err != nil {
			log.Printf("[Handshake] Oturum oluşturulamadı: %v", err)
			continue
		}

		welcome := vpn.BuildWelcomePacket(sess.VirtualIP)
		conn.WriteToUDP(welcome, clientAddr)
		log.Printf("[Handshake] %s → Sanal IP: %s (aktif: %d istemci)", clientAddr, sess.VirtualIP, sessions.Count())
	}
}

// readNetToTUN: İstemcilerden gelen şifreli paketleri çözer ve TUN'a yazar.
func readNetToTUN(conn *net.UDPConn, sessions *vpn.SessionManager, tun *water.Interface, vpnKey []byte, label string) {
	buf := make([]byte, 65535)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		decrypted, err := utils.Decrypt(buf[:n], vpnKey)
		if err != nil {
			log.Printf("[%s] GÜVENLİK: Geçersiz şifreli paket %s'den reddedildi.", label, clientAddr)
			continue
		}

		// İç paketin kaynak IP'sine bakarak istemciyi tespit et
		packet := utils.ParseIPv4(decrypted)
		if packet.SrcAddr != nil {
			// Oturumu güncelle: Bu VirtualIP'ye ait oturumun ilgili adresini güncelle
			sessions.UpdateBearerAddr(packet.SrcAddr, clientAddr, label)
		}

		tun.Write(decrypted)
	}
}
