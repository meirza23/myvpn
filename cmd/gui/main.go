package main

import (
	"fmt"
	"log"
	"myvpn/pkg/utils"
	"net"
	"sync"
	"time"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/songgao/water"
)

// ─── SABİT AYARLAR ───
// Sunucu IP'sini buradan değiştir. AWS'e geçince buraya yeni IP yaz.
const ServerIP = "192.168.64.6"

// Anahtar — Server ile aynı olmalı!
var vpnKey = []byte("12345678901234567890123456789012")

// VPN durumu
type VPNState struct {
	mu        sync.Mutex
	connected bool
	origGW    string
	origDev   string
	tunIface  *water.Interface
	stopChan  chan struct{}
}

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("MyVPN Client")
	myWindow.Resize(fyne.NewSize(460, 420))

	state := &VPNState{}

	// ─── BAŞLIK ───
	title := canvas.NewText("MyVPN", color.White)
	title.TextSize = 28
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	subtitle := canvas.NewText("AES-256-GCM Encrypted VPN", color.NRGBA{R: 150, G: 150, B: 150, A: 255})
	subtitle.TextSize = 12
	subtitle.Alignment = fyne.TextAlignCenter

	header := container.NewVBox(title, subtitle, widget.NewSeparator())

	// ─── DURUM GÖSTERGESİ ───
	statusCircle := canvas.NewCircle(color.NRGBA{R: 220, G: 50, B: 50, A: 255})
	statusCircle.Resize(fyne.NewSize(14, 14))
	statusLabel := canvas.NewText("Bağlı Değil", color.NRGBA{R: 220, G: 50, B: 50, A: 255})
	statusLabel.TextSize = 16
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	statusRow := container.NewHBox(layout.NewSpacer(), statusCircle, statusLabel, layout.NewSpacer())

	// ─── SUNUCU BİLGİSİ ───
	serverInfo := canvas.NewText("Sunucu: "+ServerIP, color.NRGBA{R: 180, G: 180, B: 180, A: 255})
	serverInfo.TextSize = 13
	serverInfo.Alignment = fyne.TextAlignCenter

	// ─── LOG PANELİ ───
	logText := widget.NewLabel("")
	logText.Wrapping = fyne.TextWrapWord
	logScroll := container.NewVScroll(logText)
	logScroll.SetMinSize(fyne.NewSize(420, 160))

	logLines := ""
	appendLog := func(msg string) {
		timestamp := time.Now().Format("15:04:05")
		logLines += fmt.Sprintf("[%s] %s\n", timestamp, msg)
		logText.SetText(logLines)
		logScroll.ScrollToBottom()
	}

	// ─── DURUM GÜNCELLEME ───
	setConnected := func(connected bool) {
		if connected {
			statusCircle.FillColor = color.NRGBA{R: 50, G: 205, B: 50, A: 255}
			statusCircle.Refresh()
			statusLabel.Text = "Bağlı"
			statusLabel.Color = color.NRGBA{R: 50, G: 205, B: 50, A: 255}
			statusLabel.Refresh()
		} else {
			statusCircle.FillColor = color.NRGBA{R: 220, G: 50, B: 50, A: 255}
			statusCircle.Refresh()
			statusLabel.Text = "Bağlı Değil"
			statusLabel.Color = color.NRGBA{R: 220, G: 50, B: 50, A: 255}
			statusLabel.Refresh()
		}
	}

	// ─── BUTONLAR ───
	var startBtn, stopBtn *widget.Button

	startBtn = widget.NewButtonWithIcon("Başlat", theme.MediaPlayIcon(), func() {
		startBtn.Disable()
		appendLog("Bağlantı kuruluyor: " + ServerIP)

		go func() {
			err := connectVPN(state, appendLog)
			if err != nil {
				appendLog("HATA: " + err.Error())
				startBtn.Enable()
				return
			}
			setConnected(true)
			stopBtn.Enable()
			appendLog("VPN bağlantısı aktif!")
		}()
	})

	stopBtn = widget.NewButtonWithIcon("Durdur", theme.MediaStopIcon(), func() {
		stopBtn.Disable()
		appendLog("Bağlantı kesiliyor...")

		go func() {
			disconnectVPN(state, appendLog)
			setConnected(false)
			startBtn.Enable()
			appendLog("VPN bağlantısı kesildi.")
		}()
	})
	stopBtn.Disable()

	buttons := container.NewGridWithColumns(2, startBtn, stopBtn)

	// ─── LOG BÖLÜMÜ ───
	logHeader := widget.NewLabel("Olay Günlüğü:")
	logHeader.TextStyle = fyne.TextStyle{Bold: true}

	// ─── ANA DÜZEN ───
	content := container.NewVBox(
		header,
		statusRow,
		serverInfo,
		widget.NewSeparator(),
		buttons,
		widget.NewSeparator(),
		logHeader,
		logScroll,
	)

	myWindow.SetContent(container.NewPadded(content))

	appendLog("Hazır — Başlat'a basın.")

	myWindow.ShowAndRun()
}

// ─── VPN BAĞLANTI FONKSİYONLARI ───

func connectVPN(state *VPNState, logFn func(string)) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.connected {
		return fmt.Errorf("zaten bağlı")
	}

	// TUN oluştur
	logFn("TUN arayüzü oluşturuluyor...")
	tun := utils.CreateTUN("10.0.0.2", "10.0.0.1", "utun5")
	logFn("TUN oluşturuldu: " + tun.Name())

	// Routing ayarla
	logFn("Routing kuralları uygulanıyor...")
	origGW, origDev := utils.SetupClientRouting(ServerIP, tun.Name())
	if origGW != "" {
		logFn(fmt.Sprintf("Orijinal gateway: %s dev %s", origGW, origDev))
	}

	state.origGW = origGW
	state.origDev = origDev
	state.tunIface = tun
	state.stopChan = make(chan struct{})
	state.connected = true

	// VPN tünelini başlat
	go runTunnel(tun, state.stopChan, logFn)

	return nil
}

func disconnectVPN(state *VPNState, logFn func(string)) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if !state.connected {
		return
	}

	if state.stopChan != nil {
		close(state.stopChan)
	}

	logFn("Routing temizleniyor...")
	utils.CleanupClientRouting(ServerIP, state.origGW, state.origDev)

	state.connected = false
	state.tunIface = nil
	logFn("Orijinal routing geri yüklendi.")
}

func runTunnel(tun *water.Interface, stop chan struct{}, logFn func(string)) {
	addrTCP, _ := net.ResolveUDPAddr("udp", ServerIP+":8080")
	connTCP, err := net.DialUDP("udp", nil, addrTCP)
	if err != nil {
		logFn("TCP bağlantı hatası: " + err.Error())
		return
	}

	addrUDP, _ := net.ResolveUDPAddr("udp", ServerIP+":8081")
	connUDP, err := net.DialUDP("udp", nil, addrUDP)
	if err != nil {
		logFn("UDP bağlantı hatası: " + err.Error())
		return
	}

	logFn("Sunucuya bağlanıldı (8080/TCP + 8081/UDP)")

	go listenNet(connTCP, tun, stop, "TCP")
	go listenNet(connUDP, tun, stop, "UDP")

	buf := make([]byte, 2000)
	for {
		select {
		case <-stop:
			connTCP.Close()
			connUDP.Close()
			logFn("Tünel kapatıldı.")
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
			continue
		}

		if packet.Protocol == utils.ProtocolTCP {
			connTCP.Write(encrypted)
		} else {
			connUDP.Write(encrypted)
		}
	}
}

func listenNet(conn *net.UDPConn, tun *water.Interface, stop chan struct{}, label string) {
	buf := make([]byte, 2000)
	for {
		select {
		case <-stop:
			return
		default:
		}

		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		decrypted, err := utils.Decrypt(buf[:n], vpnKey)
		if err != nil {
			log.Printf("[%s] Şifre çözme hatası", label)
			continue
		}

		tun.Write(decrypted)
	}
}
