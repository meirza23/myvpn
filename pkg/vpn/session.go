package vpn

import (
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	SessionTimeout = 5 * time.Minute
	vpnSubnetBase  = "10.0.0."
	maxClients     = 253 // 10.0.0.2 – 10.0.0.254

	// Handshake Magic: "MYVN"
	MagicByte0 = byte(0x4D)
	MagicByte1 = byte(0x59)
	MagicByte2 = byte(0x56)
	MagicByte3 = byte(0x4E)

	MsgHello   = byte(0x01) // İstemci → Sunucu
	MsgWelcome = byte(0x02) // Sunucu → İstemci
)

// MagicBytes handshake paketlerinin başına eklenen imza.
var MagicBytes = []byte{MagicByte0, MagicByte1, MagicByte2, MagicByte3}

// HelloPacket istemcinin handshake sırasında gönderdiği sabit paket (5 byte).
var HelloPacket = []byte{MagicByte0, MagicByte1, MagicByte2, MagicByte3, MsgHello}

// BuildWelcomePacket sunucunun istemciye atanan sanal IP ile döndürdüğü yanıt (9 byte).
func BuildWelcomePacket(ip net.IP) []byte {
	pkt := make([]byte, 9)
	copy(pkt[:4], MagicBytes)
	pkt[4] = MsgWelcome
	copy(pkt[5:9], ip.To4())
	return pkt
}

// ParseWelcomePacket Welcome paketini parse ederek atanan sanal IP'yi çıkarır.
func ParseWelcomePacket(data []byte) (net.IP, error) {
	if len(data) < 9 {
		return nil, fmt.Errorf("welcome paketi çok kısa: %d byte", len(data))
	}
	if data[0] != MagicByte0 || data[1] != MagicByte1 ||
		data[2] != MagicByte2 || data[3] != MagicByte3 {
		return nil, fmt.Errorf("geçersiz magic bytes")
	}
	if data[4] != MsgWelcome {
		return nil, fmt.Errorf("beklenmeyen mesaj tipi: 0x%02X", data[4])
	}
	return net.IP(data[5:9]).To4(), nil
}

// IsHelloPacket gelen paketin handshake Hello olup olmadığını kontrol eder.
func IsHelloPacket(data []byte) bool {
	if len(data) < 5 {
		return false
	}
	return data[0] == MagicByte0 && data[1] == MagicByte1 &&
		data[2] == MagicByte2 && data[3] == MagicByte3 &&
		data[4] == MsgHello
}

// Session bir VPN istemcisinin oturum bilgilerini tutar.
type Session struct {
	VirtualIP  net.IP
	RemoteAddr *net.UDPAddr
	LastSeen   time.Time
}

// SessionManager birden fazla VPN istemcisini güvenli biçimde yönetir.
type SessionManager struct {
	mu          sync.RWMutex
	byAddr      map[string]*Session     // remote UDP addr → session
	byVirtualIP map[string]*net.UDPAddr // virtual IP string → remote UDP addr
	usedIPs     map[int]bool            // son oktet kullanımda mı?
	nextIdx     int                     // sıradaki IP son okteti (2-254)
}

// NewSessionManager yeni bir SessionManager oluşturur.
func NewSessionManager() *SessionManager {
	return &SessionManager{
		byAddr:      make(map[string]*Session),
		byVirtualIP: make(map[string]*net.UDPAddr),
		usedIPs:     make(map[int]bool),
		nextIdx:     2,
	}
}

// Register yeni bir istemciyi kaydeder ve oturumunu döndürür.
// Zaten kayıtlıysa mevcut oturumu günceller ve döndürür.
func (sm *SessionManager) Register(remoteAddr *net.UDPAddr) (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	key := remoteAddr.String()
	if sess, ok := sm.byAddr[key]; ok {
		sess.LastSeen = time.Now()
		return sess, nil
	}

	idx, err := sm.allocateIPIndex()
	if err != nil {
		return nil, err
	}

	ip := net.ParseIP(fmt.Sprintf("%s%d", vpnSubnetBase, idx)).To4()
	sess := &Session{
		VirtualIP:  ip,
		RemoteAddr: remoteAddr,
		LastSeen:   time.Now(),
	}
	sm.byAddr[key] = sess
	sm.byVirtualIP[ip.String()] = remoteAddr
	return sess, nil
}

// GetByAddr uzak adres ile oturumu bulur.
func (sm *SessionManager) GetByAddr(remoteAddr *net.UDPAddr) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	sess, ok := sm.byAddr[remoteAddr.String()]
	return sess, ok
}

// UpdateLastSeen belirtilen uzak adresin LastSeen alanını günceller.
func (sm *SessionManager) UpdateLastSeen(remoteAddr *net.UDPAddr) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sess, ok := sm.byAddr[remoteAddr.String()]; ok {
		sess.LastSeen = time.Now()
	}
}

// GetAddrByVirtualIP sanal IP'ye karşılık gelen uzak adresi döndürür.
func (sm *SessionManager) GetAddrByVirtualIP(ip net.IP) (*net.UDPAddr, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	ip4 := ip.To4()
	if ip4 == nil {
		return nil, false
	}
	addr, ok := sm.byVirtualIP[ip4.String()]
	return addr, ok
}

// Remove bir istemci oturumunu kaldırır ve IP'yi serbest bırakır.
func (sm *SessionManager) Remove(remoteAddr *net.UDPAddr) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	key := remoteAddr.String()
	if sess, ok := sm.byAddr[key]; ok {
		ip4 := sess.VirtualIP.To4()
		if ip4 != nil {
			sm.usedIPs[int(ip4[3])] = false
		}
		delete(sm.byVirtualIP, sess.VirtualIP.String())
		delete(sm.byAddr, key)
	}
}

// CleanupExpired zaman aşımına uğramış oturumları temizler; temizlenen sayısını döndürür.
func (sm *SessionManager) CleanupExpired() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	count := 0
	for key, sess := range sm.byAddr {
		if time.Since(sess.LastSeen) > SessionTimeout {
			ip4 := sess.VirtualIP.To4()
			if ip4 != nil {
				sm.usedIPs[int(ip4[3])] = false
			}
			delete(sm.byVirtualIP, sess.VirtualIP.String())
			delete(sm.byAddr, key)
			count++
		}
	}
	return count
}

// Count aktif oturum sayısını döndürür.
func (sm *SessionManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.byAddr)
}

// allocateIPIndex boş bir sanal IP son okteti bulur.
func (sm *SessionManager) allocateIPIndex() (int, error) {
	for i := 0; i < maxClients; i++ {
		idx := ((sm.nextIdx - 2 + i) % maxClients) + 2
		if !sm.usedIPs[idx] {
			sm.usedIPs[idx] = true
			sm.nextIdx = idx + 1
			if sm.nextIdx > 254 {
				sm.nextIdx = 2
			}
			return idx, nil
		}
	}
	return 0, fmt.Errorf("sanal IP havuzu dolu (maksimum %d istemci)", maxClients)
}
