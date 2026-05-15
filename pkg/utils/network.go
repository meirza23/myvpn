package utils

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"

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

// ExtractIPv4 strips the PI header if present (macOS utun)
func ExtractIPv4(packet []byte) []byte {
	if len(packet) >= 24 && packet[0] == 0x00 && (packet[4]&0xF0) == 0x40 {
		return packet[4:]
	}
	return packet
}

// PrependPI adds the PI header if required by the OS (macOS)
func PrependPI(packet []byte) []byte {
	if runtime.GOOS == "darwin" {
		pi := []byte{0, 0, 0, 2} // AF_INET
		return append(pi, packet...)
	}
	return packet
}

func ParseIPv4(packet []byte) IPv4 {
	packet = ExtractIPv4(packet)
	// IPv4 başlığı en az 20 byte — kısa paket gelirse boş struct dön (panic önlemi)
	if len(packet) < 20 {
		return IPv4{}
	}
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

// CreateTUN yeni bir TUN arayüzü oluşturur ve IP adresini yapılandırır.
// ipAddr "10.0.0.1/24" gibi CIDR formatında ise subnet modunda eklenir
// (sunucu için multi-client desteği). Aksi halde peer modunda eklenir
// (istemci, point-to-point).
// Hata durumunda (ör. yetersiz yetki) nil ve error döndürür; log.Fatal kullanmaz.
func CreateTUN(ipAddr string, peer string, tunName string) (*water.Interface, error) {
	cfg := water.Config{
		DeviceType: water.TUN,
	}

	iface, err := water.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("TUN arayüzü oluşturulamadı (sudo ile çalıştırdığınızdan emin olun): %w", err)
	}

	log.Println("TUN arayüzü tahsis edildi:", iface.Name())

	var addrCmd []string
	if strings.Contains(ipAddr, "/") {
		// CIDR modu: tüm subnet için route otomatik oluşur (sunucu için)
		addrCmd = []string{"ip", "addr", "add", ipAddr, "dev", iface.Name()}
	} else {
		// Point-to-point modu (istemci için)
		addrCmd = []string{"ip", "addr", "add", ipAddr, "peer", peer, "dev", iface.Name()}
	}

	cmds := [][]string{
		{"ip", "link", "set", "dev", iface.Name(), "up"},
		addrCmd,
	}

	for _, cmd := range cmds {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("komut hatası %v: %w (çıktı: %s)", cmd, err, string(out))
		}
	}

	log.Printf("TUN %s yapılandırıldı: %s (peer: %s)", iface.Name(), ipAddr, peer)
	return iface, nil
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
// Kritik komutlar (forwarding ve MASQUERADE) başarısız olursa log.Fatal yapılır;
// bu kurallar olmadan VPN trafiği sessizce kaybolur.
func SetupServerRouting(tunName string, outIface string) {
	log.Println("[Routing] Server routing kuralları uygulanıyor...")

	// 1. IP Forwarding aç — KRİTİK
	if err := runCmd("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		log.Fatalf("[Routing] KRİTİK: IP forwarding açılamadı (sudo ile çalıştırın): %v", err)
	}

	// 2. NAT (MASQUERADE) - VPN subnet'inden çıkan trafik internete çıkabilsin — KRİTİK
	// -C ile var olup olmadığını kontrol et, yoksa ekle (idempotent)
	if err := runCmd("iptables", "-t", "nat", "-C", "POSTROUTING", "-s", "10.0.0.0/24", "-o", outIface, "-j", "MASQUERADE"); err != nil {
		if err := runCmd("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", "10.0.0.0/24", "-o", outIface, "-j", "MASQUERADE"); err != nil {
			log.Fatalf("[Routing] KRİTİK: MASQUERADE eklenemedi (out_iface='%s' doğru mu?): %v", outIface, err)
		}
	}

	// 3. FORWARD zincirinde TUN trafiğine izin ver — KRİTİK
	if err := runCmd("iptables", "-C", "FORWARD", "-i", tunName, "-j", "ACCEPT"); err != nil {
		if err := runCmd("iptables", "-A", "FORWARD", "-i", tunName, "-j", "ACCEPT"); err != nil {
			log.Fatalf("[Routing] KRİTİK: FORWARD -i kuralı eklenemedi: %v", err)
		}
	}
	if err := runCmd("iptables", "-C", "FORWARD", "-o", tunName, "-j", "ACCEPT"); err != nil {
		if err := runCmd("iptables", "-A", "FORWARD", "-o", tunName, "-j", "ACCEPT"); err != nil {
			log.Fatalf("[Routing] KRİTİK: FORWARD -o kuralı eklenemedi: %v", err)
		}
	}

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

	// 3. DNS'i public sunuculara geçir (yerel resolver TUN'a düşmesin)
	SetupClientDNS()

	log.Printf("[Routing] Tüm trafik %s üzerinden yönlendirildi.", tunName)
	return origGW, origDev
}

// CleanupClientRouting: Orijinal default route'u geri yükler.
func CleanupClientRouting(serverIP string, origGW string, origDev string) {
	// DNS her zaman geri yüklenmeli, route bilgileri eksik olsa bile.
	RestoreClientDNS()

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

// ============================================================
//  DNS YÖNETİMİ
// ============================================================
// Default route TUN'a yönlendirildiğinde, /etc/resolv.conf yerel
// resolver'ı (örn. 192.168.1.1) işaret ediyorsa DNS sorguları
// VPN sunucusu üzerinden o adrese gönderilir → erişilemez.
// Bu yüzden bağlantı sırasında public DNS sunucularına geçici olarak
// geçiyoruz, disconnect'te orijinali geri yüklüyoruz.

const (
	resolvConfPath   = "/etc/resolv.conf"
	resolvBackupPath = "/etc/resolv.conf.myvpn-backup"
)

// SetupClientDNS resolv.conf'u public DNS'lere yönlendirir, orijinali yedekler.
// resolv.conf systemd-resolved tarafından yönetilen bir symlink ise içerik
// üzerine yazılır (geçici olarak kalıcı dosya haline gelir); disconnect'te geri yüklenir.
func SetupClientDNS() {
	log.Println("[DNS] resolv.conf yedekleniyor ve public DNS'lere geçiliyor...")

	// Yedek zaten yoksa oluştur
	if _, err := os.Stat(resolvBackupPath); os.IsNotExist(err) {
		// Symlink çözümü için os.ReadFile kullanılır (symlink'i takip eder)
		if data, err := os.ReadFile(resolvConfPath); err == nil {
			if err := os.WriteFile(resolvBackupPath, data, 0644); err != nil {
				log.Printf("[DNS] UYARI: yedek oluşturulamadı: %v", err)
			}
		}
	}

	// Eğer resolv.conf bir symlink ise sil ki düz dosya yazabilelim
	if info, err := os.Lstat(resolvConfPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		os.Remove(resolvConfPath)
	}

	content := "# MyVPN tarafından geçici olarak ayarlandı\nnameserver 1.1.1.1\nnameserver 8.8.8.8\n"
	if err := os.WriteFile(resolvConfPath, []byte(content), 0644); err != nil {
		log.Printf("[DNS] UYARI: resolv.conf yazılamadı: %v (DNS sorguları başarısız olabilir)", err)
		return
	}
	log.Println("[DNS] Public DNS aktif: 1.1.1.1, 8.8.8.8")
}

// RestoreClientDNS yedeklenmiş resolv.conf'u geri yükler.
func RestoreClientDNS() {
	data, err := os.ReadFile(resolvBackupPath)
	if err != nil {
		// Yedek yoksa sessiz dön (zaten override edilmemiş olabilir)
		return
	}
	if err := os.WriteFile(resolvConfPath, data, 0644); err != nil {
		log.Printf("[DNS] UYARI: orijinal resolv.conf geri yüklenemedi: %v", err)
		return
	}
	os.Remove(resolvBackupPath)
	log.Println("[DNS] Orijinal resolv.conf geri yüklendi.")
}
