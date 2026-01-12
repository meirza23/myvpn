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
	cfg := water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: tunName,
		},
	}

	iface, err := water.New(cfg)
	if err != nil {
		log.Fatal("Failed to create TUN:", err)
	}

	log.Println("Allocated TUN interface:", iface.Name())

	// MacOS için syntax (Linux ise ifconfig <dev> <ip> netmask <maske> up olabilir)
	// MacOS syntax: ifconfig utunX 10.0.0.1 10.0.0.2 up
	cmds := [][]string{
		{"ifconfig", iface.Name(), ipAddr, peer, "up"},
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
