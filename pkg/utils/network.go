package utils

import (
	"encoding/binary"
	"log"
	"net"
	"os/exec"

	"github.com/songgao/water"
)

// Protokol numaraları
const (
	ProtocolICMP = 1
	ProtocolTCP  = 6
	ProtocolUDP  = 17
)

type IPv4 struct {
	Version        uint8
	IHL            uint8
	DSCP           uint8
	ECN            uint8
	TotalLength    uint16
	Identification uint16
	Flags          uint8
	FragmentOffset uint16
	TimeToLive     uint8
	Protocol       uint8
	HeaderChecksum uint16
	SrcAddr        net.IP
	DestAddr       net.IP
}

func ParseIPv4(packet []byte) IPv4 {
	return IPv4{
		Version:        (packet[0] & 0xF0) >> 4,
		IHL:            (packet[0] & 0x0F),
		DSCP:           (packet[1] & 0b11111100) >> 2,
		ECN:            (packet[1] & 0x03),
		TotalLength:    binary.BigEndian.Uint16(packet[2:4]),
		Identification: binary.BigEndian.Uint16(packet[4:6]),
		Flags:          (packet[6] & 0b11100000) >> 5,
		FragmentOffset: binary.BigEndian.Uint16(packet[6:8]) & 0x1FFF,
		TimeToLive:     packet[8],
		Protocol:       packet[9], // 6 -> TCP, 17 -> UDP
		HeaderChecksum: binary.BigEndian.Uint16(packet[10:12]),
		SrcAddr:        net.IP(packet[12:16]),
		DestAddr:       net.IP(packet[16:20]),
	}
}

func CreateTUN(ipAddr string, peer string, tunName string) *water.Interface {
	// Linux için Interface ayarı (Linux'ta isim genelde tun0, tun1 olur ama isim zorlamayalım)
	config := water.Config{
		DeviceType: water.TUN,
	}
	// water kütüphanesi Linux'ta ismi kendi de atayabilir,
	// ama sabit isim istiyorsan config.PlatformSpecificParams.Name kullanabilirsin.
	// Şimdilik basit tutalım, Linux kendi isimlendirsin (tun0 vb.)

	iface, err := water.New(config)
	if err != nil {
		log.Fatal("Failed to create TUN:", err)
	}

	log.Println("Allocated TUN interface:", iface.Name())

	// --- BURASI DEĞİŞTİ (LINUX İÇİN IP KOMUTLARI) ---
	// Eski (Mac): ifconfig utunX 10.0.0.1 10.0.0.2 up
	// Yeni (Linux): ip link set dev tun0 up && ip addr add 10.0.0.1 peer 10.0.0.2 dev tun0

	cmds := [][]string{
		{"ip", "link", "set", "dev", iface.Name(), "up"},
		{"ip", "addr", "add", ipAddr, "peer", peer, "dev", iface.Name()},
	}

	for _, cmd := range cmds {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			log.Fatalf("Failed to run %v: %v, output: %s", cmd, err, string(out))
		}
	}

	log.Printf("TUN %s configured with %s <-> %s", iface.Name(), ipAddr, peer)
	return iface
}

// runCmd: Komutu çalıştırır, hata varsa loglar ama fatal yapmaz.
func runCmd(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		log.Printf("Command failed: %s %v -> %v, output: %s", name, args, err, string(out))
	}
	return err
}

// ============================================================
//  SERVER ROUTING
// ============================================================

// SetupServerRouting: IP forwarding açar ve NAT (masquerade) kurallarını ekler.
// outIface: sunucunun internet çıkış arayüzü (ör. eth0, ens33)
func SetupServerRouting(tunName string, outIface string) {
	log.Println("[Routing] Server routing kuralları uygulanıyor...")

	// 1. IP Forwarding aç
	runCmd("sysctl", "-w", "net.ipv4.ip_forward=1")

	// 2. NAT (MASQUERADE) - VPN subnet'inden çıkan trafik internete çıkabilsin
	runCmd("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", "10.0.0.0/24", "-o", outIface, "-j", "MASQUERADE")

	// 3. FORWARD zincirinde TUN trafiğine izin ver
	runCmd("iptables", "-A", "FORWARD", "-i", tunName, "-j", "ACCEPT")
	runCmd("iptables", "-A", "FORWARD", "-o", tunName, "-j", "ACCEPT")

	log.Println("[Routing] Server routing aktif.")
}

// CleanupServerRouting: Eklenen iptables kurallarını kaldırır.
func CleanupServerRouting(tunName string, outIface string) {
	log.Println("[Routing] Server routing kuralları temizleniyor...")
	runCmd("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", "10.0.0.0/24", "-o", outIface, "-j", "MASQUERADE")
	runCmd("iptables", "-D", "FORWARD", "-i", tunName, "-j", "ACCEPT")
	runCmd("iptables", "-D", "FORWARD", "-o", tunName, "-j", "ACCEPT")
	log.Println("[Routing] Server routing temizlendi.")
}

// ============================================================
//  CLIENT ROUTING
// ============================================================

// GetDefaultGateway: Mevcut default gateway IP ve arayüz adını döndürür.
func GetDefaultGateway() (gatewayIP string, devName string) {
	out, err := exec.Command("ip", "route", "show", "default").CombinedOutput()
	if err != nil {
		log.Println("Default gateway alınamadı:", err)
		return "", ""
	}
	// Çıktı formatı: "default via 192.168.1.1 dev eth0 ..."
	fields := splitFields(string(out))
	for i, f := range fields {
		if f == "via" && i+1 < len(fields) {
			gatewayIP = fields[i+1]
		}
		if f == "dev" && i+1 < len(fields) {
			devName = fields[i+1]
		}
	}
	return gatewayIP, devName
}

// splitFields: Boşluk ve newline'a göre string'i parçalar.
func splitFields(s string) []string {
	var fields []string
	current := ""
	for _, c := range s {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if current != "" {
				fields = append(fields, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		fields = append(fields, current)
	}
	return fields
}

// SetupClientRouting: Tüm trafiği VPN tüneline yönlendirir.
// serverIP: VPN sunucusunun gerçek IP adresi
// tunName: TUN arayüzünün adı
// Döndürür: orijinal gateway IP ve device (cleanup için saklanmalı)
func SetupClientRouting(serverIP string, tunName string) (origGW string, origDev string) {
	origGW, origDev = GetDefaultGateway()
	log.Printf("[Routing] Mevcut gateway: %s dev %s", origGW, origDev)

	if origGW == "" || origDev == "" {
		log.Println("[Routing] UYARI: Default gateway bulunamadı, routing atlanıyor.")
		return "", ""
	}

	// 1. Server IP'ye direkt rota ekle (VPN bağlantısı kopmasın)
	runCmd("ip", "route", "add", serverIP+"/32", "via", origGW, "dev", origDev)

	// 2. Default route'u TUN'a yönlendir
	runCmd("ip", "route", "replace", "default", "dev", tunName)

	log.Printf("[Routing] Tüm trafik %s üzerinden yönlendirildi.", tunName)
	return origGW, origDev
}

// CleanupClientRouting: Orijinal default route'u geri yükler.
func CleanupClientRouting(serverIP string, origGW string, origDev string) {
	if origGW == "" || origDev == "" {
		return
	}
	log.Println("[Routing] Client routing temizleniyor...")

	// 1. Orijinal default route'u geri getir
	runCmd("ip", "route", "replace", "default", "via", origGW, "dev", origDev)

	// 2. Server'a eklenen host route'u kaldır
	runCmd("ip", "route", "del", serverIP+"/32")

	log.Println("[Routing] Orijinal routing geri yüklendi.")
}
