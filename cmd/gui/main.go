package main

import (
	"fmt"
	"image/color"
	"log"
	"myvpn/pkg/config"
	"myvpn/pkg/utils"
	"myvpn/pkg/vpn"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/songgao/water"
)

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
	// Ayarları yükle
	cfg := config.LoadClientConfig()

	myApp := app.NewWithID("com.myvpn.client")
	myApp.Settings().SetTheme(theme.DarkTheme())

	myWindow := myApp.NewWindow("MyVPN")
	myWindow.Resize(fyne.NewSize(480, 560))
	myWindow.SetFixedSize(true)

	state := &VPNState{}

	// ─── BAŞLIK ───
	title := canvas.NewText("⬡ MyVPN", color.NRGBA{R: 99, G: 179, B: 237, A: 255})
	title.TextSize = 32
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	subtitle := canvas.NewText("AES-256-GCM • "+cfg.ServerIP, color.NRGBA{R: 113, G: 128, B: 150, A: 255})
	subtitle.TextSize = 12
	subtitle.Alignment = fyne.TextAlignCenter

	// ─── DURUM GÖSTERGESİ ───
	statusDot := canvas.NewCircle(color.NRGBA{R: 245, G: 101, B: 101, A: 255})
	statusDot.Resize(fyne.NewSize(12, 12))
	statusText := canvas.NewText("Bağlı Değil", color.NRGBA{R: 245, G: 101, B: 101, A: 255})
	statusText.TextSize = 18
	statusText.TextStyle = fyne.TextStyle{Bold: true}
	statusRow := container.NewHBox(layout.NewSpacer(), statusDot, statusText, layout.NewSpacer())

	// ─── DATA BINDINGS ───
	timerBind := binding.NewString()
	sentBind := binding.NewString()
	recvBind := binding.NewString()
	logBind := binding.NewString()

	_ = timerBind.Set("")
	_ = sentBind.Set("↑ 0 B")
	_ = recvBind.Set("↓ 0 B")
	_ = logBind.Set("")

	timerLabel := widget.NewLabelWithData(timerBind)
	timerLabel.Alignment = fyne.TextAlignCenter
	sentLabel := widget.NewLabelWithData(sentBind)
	recvLabel := widget.NewLabelWithData(recvBind)
	statsRow := container.NewHBox(layout.NewSpacer(), sentLabel, widget.NewLabel("   "), recvLabel, layout.NewSpacer())

	// ─── LOG (mutex korumalı, goroutine-safe) ───
	var logMu sync.Mutex
	logLines := ""
	appendLog := func(msg string) {
		logMu.Lock()
		defer logMu.Unlock()
		ts := time.Now().Format("15:04:05")
		logLines += fmt.Sprintf("[%s] %s\n", ts, msg)
		_ = logBind.Set(logLines)
	}

	logText := widget.NewLabelWithData(logBind)
	logText.Wrapping = fyne.TextWrapWord
	logScroll := container.NewVScroll(logText)
	logScroll.SetMinSize(fyne.NewSize(440, 120))

	// ─── PROGRESS BAR ───
	progress := widget.NewProgressBarInfinite()
	progress.Hide()

	// ─── DURUM GÜNCELLEMESİ ───
	var timerStop chan struct{}

	setConnected := func(connected bool) {
		if connected {
			statusDot.FillColor = color.NRGBA{R: 72, G: 199, B: 142, A: 255}
			statusDot.Refresh()
			statusText.Text = "Bağlı"
			statusText.Color = color.NRGBA{R: 72, G: 199, B: 142, A: 255}
			statusText.Refresh()

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
						_ = timerBind.Set(fmt.Sprintf("⏱  %s", elapsed))
						_ = sentBind.Set("↑ " + formatBytes(state.bytesSent.Load()))
						_ = recvBind.Set("↓ " + formatBytes(state.bytesRecv.Load()))
					}
				}
			}()
		} else {
			statusDot.FillColor = color.NRGBA{R: 245, G: 101, B: 101, A: 255}
			statusDot.Refresh()
			statusText.Text = "Bağlı Değil"
			statusText.Color = color.NRGBA{R: 245, G: 101, B: 101, A: 255}
			statusText.Refresh()
			_ = timerBind.Set("")
			_ = sentBind.Set("↑ 0 B")
			_ = recvBind.Set("↓ 0 B")

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
		appendLog("Bağlantı kuruluyor → " + cfg.ServerIP)

		go func() {
			err := connectVPN(state, cfg, appendLog)
			fyne.Do(func() {
				progress.Hide()
				if err != nil {
					appendLog("HATA: " + err.Error())
					dialog.ShowError(err, myWindow)
					startBtn.Enable()
					return
				}
				setConnected(true)
				stopBtn.Enable()
				appendLog("VPN aktif!")
			})
		}()
	})
	startBtn.Importance = widget.HighImportance

	stopBtn = widget.NewButtonWithIcon("  Kes", theme.MediaStopIcon(), func() {
		stopBtn.Disable()
		appendLog("Bağlantı kesiliyor...")
		go func() {
			disconnectVPN(state, cfg, appendLog)
			fyne.Do(func() {
				setConnected(false)
				startBtn.Enable()
				appendLog("VPN kapatıldı.")
			})
		}()
	})
	stopBtn.Disable()

	buttons := container.NewGridWithColumns(2, startBtn, stopBtn)
	logHeader := canvas.NewText("Olay Günlüğü", color.NRGBA{R: 113, G: 128, B: 150, A: 255})
	logHeader.TextSize = 11

	// ─── BAĞLAN SEKMESİ ───
	connectTab := container.NewVBox(
		container.NewPadded(container.NewVBox(title, subtitle)),
		widget.NewSeparator(),
		container.NewPadded(container.NewVBox(statusRow, timerLabel, statsRow)),
		widget.NewSeparator(),
		container.NewPadded(buttons),
		progress,
		widget.NewSeparator(),
		container.NewPadded(container.NewVBox(logHeader, logScroll)),
	)

	// ─── AYARLAR SEKMESİ ───
	serverIPEntry := widget.NewEntry()
	serverIPEntry.SetText(cfg.ServerIP)
	serverIPEntry.SetPlaceHolder("Örn: 192.168.1.100")

	portEntry := widget.NewEntry()
	portEntry.SetText(fmt.Sprintf("%d", cfg.Port))
	portEntry.SetPlaceHolder("Varsayılan: 8079")

	keyEntry := widget.NewPasswordEntry()
	keyEntry.SetText(cfg.VPNKey)
	keyEntry.SetPlaceHolder("Tam 32 karakter AES-256 anahtarı")

	keyInfo := canvas.NewText("⚠ Sunucu ile aynı anahtar kullanılmalı", color.NRGBA{R: 237, G: 137, B: 54, A: 255})
	keyInfo.TextSize = 11

	saveBtn := widget.NewButtonWithIcon("  Kaydet", theme.DocumentSaveIcon(), func() {
		ip := serverIPEntry.Text
		portStr := portEntry.Text
		key := keyEntry.Text

		// Validasyon
		if net.ParseIP(ip) == nil {
			dialog.ShowError(fmt.Errorf("geçersiz IP adresi: %s", ip), myWindow)
			return
		}
		port := 0
		if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil || port < 1 || port > 65535 {
			dialog.ShowError(fmt.Errorf("geçersiz port numarası: %s", portStr), myWindow)
			return
		}
		if len(key) != 32 {
			dialog.ShowError(fmt.Errorf("VPN anahtarı tam 32 karakter olmalı (şu an: %d karakter)", len(key)), myWindow)
			return
		}

		cfg.ServerIP = ip
		cfg.Port = port
		cfg.VPNKey = key

		if err := cfg.Save(); err != nil {
			dialog.ShowError(fmt.Errorf("ayarlar kaydedilemedi: %w", err), myWindow)
			return
		}

		// Subtitle'ı güncelle
		subtitle.Text = "AES-256-GCM • " + cfg.ServerIP
		subtitle.Refresh()

		dialog.ShowInformation("Kaydedildi", "Ayarlar başarıyla kaydedildi.\nYeni sunucu: "+cfg.ServerIP, myWindow)
		appendLog("Ayarlar güncellendi → " + cfg.ServerIP)
	})
	saveBtn.Importance = widget.HighImportance

	settingsForm := widget.NewForm(
		widget.NewFormItem("Sunucu IP", serverIPEntry),
		widget.NewFormItem("Kontrol Portu", portEntry),
		widget.NewFormItem("VPN Anahtarı", keyEntry),
	)

	settingsTab := container.NewVBox(
		container.NewPadded(settingsForm),
		container.NewPadded(keyInfo),
		widget.NewSeparator(),
		container.NewPadded(saveBtn),
	)

	// ─── TAB CONTAINER ───
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Bağlan", theme.MediaPlayIcon(), connectTab),
		container.NewTabItemWithIcon("Ayarlar", theme.SettingsIcon(), settingsTab),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	myWindow.SetContent(tabs)

	// Kullanıcı pencereyi kapatırken bağlıysa routing/TUN/DNS temizliği yap.
	// Aksi halde sistem bozuk routing kuralları ile kalır (internet "kopuk" görünür).
	myWindow.SetCloseIntercept(func() {
		if state.connected {
			appendLog("Pencere kapatılıyor — VPN temizleniyor...")
			disconnectVPN(state, cfg, appendLog)
		}
		myWindow.Close()
	})

	appendLog("Hazır — Bağlan'a basın.")
	myWindow.ShowAndRun()
}

// ─── YARDIMCI ───
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

func connectVPN(state *VPNState, cfg *config.ClientConfig, logFn func(string)) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.connected {
		return fmt.Errorf("zaten bağlı")
	}

	vpnKey := []byte(cfg.VPNKey)
	if len(vpnKey) != 32 {
		return fmt.Errorf("VPN anahtarı 32 karakter olmalı (şu an: %d). Ayarlar sekmesini kontrol edin", len(vpnKey))
	}

	// 1. Handshake — sunucudan sanal IP al
	logFn("Sunucuya bağlanılıyor (handshake)...")
	serverAddr := fmt.Sprintf("%s:%d", cfg.ServerIP, cfg.Port)
	assignedIP, err := performHandshake(serverAddr)
	if err != nil {
		return fmt.Errorf("handshake başarısız: %w", err)
	}
	logFn(fmt.Sprintf("Sanal IP atandı: %s", assignedIP))

	// 2. TUN arayüzü oluştur
	logFn("TUN arayüzü oluşturuluyor...")
	tun, err := utils.CreateTUN(assignedIP.String(), "10.0.0.1", "")
	if err != nil {
		return err
	}
	logFn("TUN: " + tun.Name())

	// 3. Routing ayarla
	logFn("Routing ayarlanıyor...")
	origGW, origDev := utils.SetupClientRouting(cfg.ServerIP, tun.Name())

	state.origGW = origGW
	state.origDev = origDev
	state.tunIface = tun
	state.stopChan = make(chan struct{})
	state.startTime = time.Now()
	state.bytesSent.Store(0)
	state.bytesRecv.Store(0)
	state.connected = true

	// logFn goroutine'den çağrıldığında fyne thread'ine ilet
	safeLog := func(msg string) {
		fyne.Do(func() { logFn(msg) })
	}

	go runTunnel(tun, cfg.ServerIP, vpnKey, state.stopChan, &state.bytesSent, &state.bytesRecv, safeLog)
	return nil
}

func disconnectVPN(state *VPNState, cfg *config.ClientConfig, logFn func(string)) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if !state.connected {
		return
	}
	// Stop sinyali — nil yaparak ikinci disconnect'te double-close panic'i engellenir.
	if state.stopChan != nil {
		close(state.stopChan)
		state.stopChan = nil
	}
	// TUN'u routing temizliğinden ÖNCE kapat — bloklayıcı tun.Read() çağrıları çıksın.
	if state.tunIface != nil {
		state.tunIface.Close()
		state.tunIface = nil
	}
	utils.CleanupClientRouting(cfg.ServerIP, state.origGW, state.origDev)
	state.connected = false
	logFn("Routing temizlendi.")
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
		return nil, fmt.Errorf("welcome beklerken zaman aşımı: %w (sunucu çalışıyor mu?)", err)
	}

	return vpn.ParseWelcomePacket(buf[:n])
}

func runTunnel(tun *water.Interface, serverIP string, vpnKey []byte, stop chan struct{}, bytesSent *atomic.Uint64, bytesRecv *atomic.Uint64, logFn func(string)) {
	addrTCP, _ := net.ResolveUDPAddr("udp", serverIP+":8080")
	connTCP, err := net.DialUDP("udp", nil, addrTCP)
	if err != nil {
		logFn("TCP bağlantı hatası: " + err.Error())
		return
	}
	defer connTCP.Close()

	addrUDP, _ := net.ResolveUDPAddr("udp", serverIP+":8081")
	connUDP, err := net.DialUDP("udp", nil, addrUDP)
	if err != nil {
		logFn("UDP bağlantı hatası: " + err.Error())
		return
	}
	defer connUDP.Close()

	logFn("Sunucuya veri kanalı açıldı ✓")

	go listenNet(connTCP, tun, stop, "TCP", vpnKey, bytesRecv)
	go listenNet(connUDP, tun, stop, "UDP", vpnKey, bytesRecv)

	buf := make([]byte, 65535)
	for {
		select {
		case <-stop:
			return
		default:
		}

		n, err := tun.Read(buf)
		if err != nil {
			// TUN kapatıldıysa Read hata döner — stop'u kontrol edip çık.
			select {
			case <-stop:
				return
			default:
				continue
			}
		}

		rawIPv4 := utils.ExtractIPv4(buf[:n])
		packet := utils.ParseIPv4(rawIPv4)
		encrypted, err := utils.Encrypt(rawIPv4, vpnKey)
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

func listenNet(conn *net.UDPConn, tun *water.Interface, stop chan struct{}, label string, vpnKey []byte, bytesRecv *atomic.Uint64) {
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
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // stop channel kontrolü için döngü başına dön
			}
			return // gerçek hata (bağlantı kapatıldı)
		}

		decrypted, err := utils.Decrypt(buf[:n], vpnKey)
		if err != nil {
			log.Printf("[%s] şifre çözme hatası", label)
			continue
		}

		tun.Write(utils.PrependPI(decrypted))
		bytesRecv.Add(uint64(n))
	}
}
