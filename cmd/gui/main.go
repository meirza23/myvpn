package main

import (
	"fmt"
	"log"
	"myvpn/pkg/utils"
	"net"
	"sync"
	"sync/atomic"
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
const ServerIP = "192.168.64.6"

var vpnKey = []byte("12345678901234567890123456789012")

// ─── VPN DURUMU ───
type VPNState struct {
	mu        sync.Mutex
	connected bool
	origGW    string
	origDev   string
	tunIface  *water.Interface
	stopChan  chan struct{}
	startTime time.Time
	bytesSent atomic.Uint64
	bytesRecv atomic.Uint64
}

func main() {
	myApp := app.NewWithID("com.myvpn.client")
	myApp.Settings().SetTheme(theme.DarkTheme())

	myWindow := myApp.NewWindow("MyVPN")
	myWindow.Resize(fyne.NewSize(480, 500))
	myWindow.SetFixedSize(true)

	state := &VPNState{}

	// ─── BAŞLIK ───
	title := canvas.NewText("⬡ MyVPN", color.NRGBA{R: 99, G: 179, B: 237, A: 255})
	title.TextSize = 32
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	subtitle := canvas.NewText("AES-256-GCM • "+ServerIP, color.NRGBA{R: 113, G: 128, B: 150, A: 255})
	subtitle.TextSize = 12
	subtitle.Alignment = fyne.TextAlignCenter

	// ─── DURUM GÖSTERGESI ───
	statusDot := canvas.NewCircle(color.NRGBA{R: 245, G: 101, B: 101, A: 255})
	statusDot.Resize(fyne.NewSize(12, 12))
	statusText := canvas.NewText("Bağlı Değil", color.NRGBA{R: 245, G: 101, B: 101, A: 255})
	statusText.TextSize = 18
	statusText.TextStyle = fyne.TextStyle{Bold: true}
	statusRow := container.NewHBox(layout.NewSpacer(), statusDot, statusText, layout.NewSpacer())

	// ─── BAĞLANTI SÜRESİ ───
	timerLabel := canvas.NewText("", color.NRGBA{R: 160, G: 174, B: 192, A: 255})
	timerLabel.TextSize = 13
	timerLabel.Alignment = fyne.TextAlignCenter

	// ─── VERİ İSTATİSTİKLERİ ───
	sentLabel := canvas.NewText("↑ 0 B", color.NRGBA{R: 154, G: 230, B: 180, A: 255})
	sentLabel.TextSize = 12
	recvLabel := canvas.NewText("↓ 0 B", color.NRGBA{R: 144, G: 205, B: 244, A: 255})
	recvLabel.TextSize = 12
	statsRow := container.NewHBox(layout.NewSpacer(), sentLabel, canvas.NewText("   ", color.Transparent), recvLabel, layout.NewSpacer())

	// ─── PROGRESS BAR (bağlanırken) ───
	progress := widget.NewProgressBarInfinite()
	progress.Hide()

	// ─── LOG PANELİ ───
	logText := widget.NewLabel("")
	logText.Wrapping = fyne.TextWrapWord
	logScroll := container.NewVScroll(logText)
	logScroll.SetMinSize(fyne.NewSize(440, 150))

	logLines := ""
	appendLog := func(msg string) {
		ts := time.Now().Format("15:04:05")
		logLines += fmt.Sprintf("[%s] %s\n", ts, msg)
		logText.SetText(logLines)
		logScroll.ScrollToBottom()
	}

	// ─── DURUM GÜNCELLEMESİ ───
	var timerStop chan struct{}

	setConnected := func(connected bool) {
		if connected {
			statusDot.FillColor = color.NRGBA{R: 72, G: 199, B: 142, A: 255}
			statusDot.Refresh()
			statusText.Text = "Bağlı"
			statusText.Color = color.NRGBA{R: 72, G: 199, B: 142, A: 255}
			statusText.Refresh()

			// Timer başlat
			timerStop = make(chan struct{})
			go func() {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-timerStop:
						return
					case <-ticker.C:
						elapsed := time.Since(state.startTime).Round(time.Second)
						timerLabel.Text = fmt.Sprintf("⏱  %s", elapsed)
						timerLabel.Refresh()

						sent := state.bytesSent.Load()
						recv := state.bytesRecv.Load()
						sentLabel.Text = "↑ " + formatBytes(sent)
						sentLabel.Refresh()
						recvLabel.Text = "↓ " + formatBytes(recv)
						recvLabel.Refresh()
					}
				}
			}()
		} else {
			statusDot.FillColor = color.NRGBA{R: 245, G: 101, B: 101, A: 255}
			statusDot.Refresh()
			statusText.Text = "Bağlı Değil"
			statusText.Color = color.NRGBA{R: 245, G: 101, B: 101, A: 255}
			statusText.Refresh()
			timerLabel.Text = ""
			timerLabel.Refresh()
			sentLabel.Text = "↑ 0 B"
			sentLabel.Refresh()
			recvLabel.Text = "↓ 0 B"
			recvLabel.Refresh()

			if timerStop != nil {
				close(timerStop)
				timerStop = nil
			}
		}
	}

	// ─── BUTONLAR ───
	var startBtn, stopBtn *widget.Button

	startBtn = widget.NewButtonWithIcon("  Bağlan", theme.MediaPlayIcon(), func() {
		startBtn.Disable()
		progress.Show()
		appendLog("Bağlantı kuruluyor → " + ServerIP)

		go func() {
			err := connectVPN(state, appendLog)
			progress.Hide()
			if err != nil {
				appendLog("HATA: " + err.Error())
				startBtn.Enable()
				return
			}
			setConnected(true)
			stopBtn.Enable()
			appendLog("VPN aktif!")
		}()
	})
	startBtn.Importance = widget.HighImportance

	stopBtn = widget.NewButtonWithIcon("  Kes", theme.MediaStopIcon(), func() {
		stopBtn.Disable()
		appendLog("Bağlantı kesiliyor...")
		go func() {
			disconnectVPN(state, appendLog)
			setConnected(false)
			startBtn.Enable()
			appendLog("VPN kapatıldı.")
		}()
	})
	stopBtn.Disable()

	buttons := container.NewGridWithColumns(2, startBtn, stopBtn)

	// ─── SEPARATOR & LOG HEADER ───
	logHeader := canvas.NewText("Olay Günlüğü", color.NRGBA{R: 113, G: 128, B: 150, A: 255})
	logHeader.TextSize = 11

	// ─── ANA DÜZEN ───
	content := container.NewVBox(
		container.NewPadded(container.NewVBox(title, subtitle)),
		widget.NewSeparator(),
		container.NewPadded(container.NewVBox(
			statusRow,
			timerLabel,
			statsRow,
		)),
		widget.NewSeparator(),
		container.NewPadded(buttons),
		progress,
		widget.NewSeparator(),
		container.NewPadded(container.NewVBox(logHeader, logScroll)),
	)

	myWindow.SetContent(content)
	appendLog("Hazır — Bağlan'a basın.")
	myWindow.ShowAndRun()
}

// ─── YARDIMCI: byte formatla ───
func formatBytes(b uint64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// ─── VPN BAĞLANTI FONKSİYONLARI ───

func connectVPN(state *VPNState, logFn func(string)) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.connected {
		return fmt.Errorf("zaten bağlı")
	}

	logFn("TUN arayüzü oluşturuluyor...")
	tun := utils.CreateTUN("10.0.0.2", "10.0.0.1", "utun5")
	logFn("TUN: " + tun.Name())

	logFn("Routing ayarlanıyor...")
	origGW, origDev := utils.SetupClientRouting(ServerIP, tun.Name())

	state.origGW = origGW
	state.origDev = origDev
	state.tunIface = tun
	state.stopChan = make(chan struct{})
	state.startTime = time.Now()
	state.bytesSent.Store(0)
	state.bytesRecv.Store(0)
	state.connected = true

	go runTunnel(tun, state.stopChan, &state.bytesSent, &state.bytesRecv, logFn)
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

	utils.CleanupClientRouting(ServerIP, state.origGW, state.origDev)
	state.connected = false
	state.tunIface = nil
	logFn("Routing temizlendi.")
}

func runTunnel(tun *water.Interface, stop chan struct{}, bytesSent *atomic.Uint64, bytesRecv *atomic.Uint64, logFn func(string)) {
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

	logFn("Sunucuya bağlanıldı ✓")

	go listenNet(connTCP, tun, stop, "TCP", bytesRecv)
	go listenNet(connUDP, tun, stop, "UDP", bytesRecv)

	buf := make([]byte, 2000)
	for {
		select {
		case <-stop:
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
			continue
		}

		if packet.Protocol == utils.ProtocolTCP {
			connTCP.Write(encrypted)
		} else {
			connUDP.Write(encrypted)
		}
		bytesSent.Add(uint64(n))
	}
}

func listenNet(conn *net.UDPConn, tun *water.Interface, stop chan struct{}, label string, bytesRecv *atomic.Uint64) {
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
			log.Printf("[%s] şifre çözme hatası", label)
			continue
		}

		tun.Write(decrypted)
		bytesRecv.Add(uint64(n))
	}
}
