package main

import (
	"log"
	"myvpn/pkg/utils" // go.mod'daki isme göre
	"net"
	"sync/atomic"

	"github.com/songgao/water"
)

func main() {
	// Server TUN IP: 10.0.0.1, Peer (Client): 10.0.0.2
	tun := utils.CreateTUN("10.0.0.1", "10.0.0.2", "utun6")

	// İki ayrı port dinliyoruz
	go socketServer(tun)

	select {}
}

func socketServer(tun *water.Interface) {
	// 1. Port: TCP taşıyan paketler için (Örn: 8080)
	addrTCP, _ := net.ResolveUDPAddr("udp", ":8080")
	connTCP, err := net.ListenUDP("udp", addrTCP)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Listening for TCP-embedded packets on: :8080")

	// 2. Port: UDP taşıyan paketler için (Örn: 8081)
	addrUDP, _ := net.ResolveUDPAddr("udp", ":8081")
	connUDP, err := net.ListenUDP("udp", addrUDP)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Listening for UDP-embedded packets on: :8081")

	// Client adreslerini thread-safe saklamak için
	var clientAddrForTCP atomic.Value
	var clientAddrForUDP atomic.Value

	// Ağdan geleni TUN'a yazma (Network -> TUN)
	go readFromNetToTun(connTCP, tun, &clientAddrForTCP, "TCP-Line")
	go readFromNetToTun(connUDP, tun, &clientAddrForUDP, "UDP-Line")

	// TUN'dan geleni Ağa yazma (TUN -> Network)
	// Burada paketin içine bakıp hangi yoldan göndereceğimize karar vereceğiz
	buf := make([]byte, 2000)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			log.Println("TUN Read Error:", err)
			continue
		}

		packet := utils.ParseIPv4(buf[:n])

		// Kural: Eğer paket TCP ise -> connTCP üzerinden gönder
		// Eğer paket UDP ise -> connUDP üzerinden gönder
		// Diğerleri (ICMP vs) -> Varsayılan olarak UDP hattından gitsin (Ping için)

		if packet.Protocol == utils.ProtocolTCP {
			// TCP Hattını Kullan
			addr := clientAddrForTCP.Load()
			if addr != nil {
				connTCP.WriteToUDP(buf[:n], addr.(*net.UDPAddr))
				log.Printf("[TUN->Net] Sent TCP packet via Port 8080")
			}
		} else {
			// UDP veya Diğerleri (ICMP) -> UDP Hattını Kullan (Port 8081)
			addr := clientAddrForUDP.Load()
			if addr != nil {
				connUDP.WriteToUDP(buf[:n], addr.(*net.UDPAddr))
				log.Printf("[TUN->Net] Sent UDP/ICMP packet via Port 8081")
			}
		}
	}
}

func readFromNetToTun(conn *net.UDPConn, tun *water.Interface, remoteAddr *atomic.Value, label string) {
	buf := make([]byte, 2000)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		// Client adresini güncelle (NAT vs değişirse diye)
		remoteAddr.Store(clientAddr)

		tun.Write(buf[:n])
		log.Printf("[Net->TUN] Received via %s from %s", label, clientAddr)
	}
}
