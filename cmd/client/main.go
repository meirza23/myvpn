package main

import (
	"flag"
	"log"
	"myvpn/pkg/utils"
	"net"

	"github.com/songgao/water"
)

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

	log.Println("Connected to server ports 8080 (TCP-Line) and 8081 (UDP-Line)")

	// Server'dan gelen cevapları dinle ve TUN'a yaz
	go readFromNetToTun(connTCP, tun, "TCP-Line")
	go readFromNetToTun(connUDP, tun, "UDP-Line")

	// TUN'dan oku ve Protokole göre ayır
	buf := make([]byte, 2000)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			continue
		}

		packet := utils.ParseIPv4(buf[:n])

		// KARAR MEKANİZMASI: TCP mi UDP mi?
		if packet.Protocol == utils.ProtocolTCP {
			// TCP Paketlerini 8080'e yolla
			connTCP.Write(buf[:n])
			log.Printf("[TUN->Net] Routing TCP packet to Port 8080")
		} else {
			// UDP ve ICMP Paketlerini 8081'e yolla
			connUDP.Write(buf[:n])
			log.Printf("[TUN->Net] Routing UDP/ICMP packet to Port 8081")
		}
	}
}

func readFromNetToTun(conn *net.UDPConn, tun *water.Interface, label string) {
	buf := make([]byte, 2000)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		tun.Write(buf[:n])
		log.Printf("[Net->TUN] Received via %s", label)
	}
}
