package utils

import "fmt"

// ANSI Renk Kodları
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
	ColorBold   = "\033[1m"
	ColorDim    = "\033[2m"
)

// PrintBanner: Uygulama başlangıcında gösterilecek ASCII banner.
func PrintBanner() {
	banner := ColorCyan + ColorBold + `
  ╔══════════════════════════════════╗
  ║                                  ║
  ║     ███╗   ███╗██╗   ██╗        ║
  ║     ████╗ ████║╚██╗ ██╔╝        ║
  ║     ██╔████╔██║ ╚████╔╝         ║
  ║     ██║╚██╔╝██║  ╚██╔╝          ║
  ║     ██║ ╚═╝ ██║   ██║           ║
  ║     ╚═╝     ╚═╝   ╚═╝           ║
  ║        MyVPN Client v1.0         ║
  ║    AES-256-GCM Encrypted VPN     ║
  ║                                  ║
  ╚══════════════════════════════════╝
` + ColorReset
	fmt.Println(banner)
}

// PrintStatus: Bağlantı durumunu renkli gösterir.
func PrintStatus(connected bool, serverIP string) {
	fmt.Println()
	if connected {
		fmt.Printf("  %s● Bağlantı: %sAKTİF%s\n", ColorGreen, ColorBold, ColorReset)
		fmt.Printf("  %s⬡ Sunucu:   %s%s\n", ColorDim, serverIP, ColorReset)
		fmt.Printf("  %s⬡ Şifreleme: AES-256-GCM%s\n", ColorDim, ColorReset)
		fmt.Printf("  %s⬡ Portlar:   8080 (TCP) / 8081 (UDP)%s\n", ColorDim, ColorReset)
	} else {
		fmt.Printf("  %s● Bağlantı: %sBağlı Değil%s\n", ColorRed, ColorBold, ColorReset)
	}
	fmt.Println()
}

// PrintMenu: Menü seçeneklerini gösterir.
func PrintMenu(connected bool) {
	fmt.Println(ColorCyan + "  ─────────────────────────────────" + ColorReset)
	if !connected {
		fmt.Println("  " + ColorBold + "1" + ColorReset + ") " + ColorGreen + "Bağlan (Connect)" + ColorReset)
	} else {
		fmt.Println("  " + ColorBold + "1" + ColorReset + ") " + ColorRed + "Bağlantıyı Kes (Disconnect)" + ColorReset)
	}
	fmt.Println("  " + ColorBold + "2" + ColorReset + ") Durum (Status)")
	fmt.Println("  " + ColorBold + "3" + ColorReset + ") Çıkış (Quit)")
	fmt.Println(ColorCyan + "  ─────────────────────────────────" + ColorReset)
	fmt.Print("\n  " + ColorYellow + "Seçim> " + ColorReset)
}

// PrintInfo: Bilgi mesajı yazdırır.
func PrintInfo(msg string) {
	fmt.Printf("  %s[i]%s %s\n", ColorCyan, ColorReset, msg)
}

// PrintSuccess: Başarı mesajı yazdırır.
func PrintSuccess(msg string) {
	fmt.Printf("  %s[✓]%s %s\n", ColorGreen, ColorReset, msg)
}

// PrintError: Hata mesajı yazdırır.
func PrintError(msg string) {
	fmt.Printf("  %s[✗]%s %s\n", ColorRed, ColorReset, msg)
}

// PrintWarning: Uyarı mesajı yazdırır.
func PrintWarning(msg string) {
	fmt.Printf("  %s[!]%s %s\n", ColorYellow, ColorReset, msg)
}
