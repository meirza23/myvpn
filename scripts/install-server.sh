#!/bin/bash
# ============================================
#  MyVPN Server — Otomatik Kurulum Scripti
#  AWS/VPS üzerinde bir kere çalıştır, sonra unut.
# ============================================

set -e

echo "═══════════════════════════════════"
echo "  MyVPN Server Kurulumu Başlıyor"
echo "═══════════════════════════════════"

# 1. Go yüklü mü kontrol et
if ! command -v go &> /dev/null; then
    echo "[*] Go kuruluyor..."
    sudo snap install go --classic
fi

# 2. Projeyi derle → tek binary dosya
echo "[*] Server derleniyor..."
cd "$(dirname "$0")/.."
sudo go build -o /usr/local/bin/myvpn-server ./cmd/server/
echo "[✓] Binary: /usr/local/bin/myvpn-server"

# 3. Ağ arayüzünü otomatik bul
IFACE=$(ip route show default | awk '{print $5; exit}')
echo "[✓] Çıkış arayüzü: $IFACE"

# 4. Systemd servis dosyası oluştur
echo "[*] Systemd servisi oluşturuluyor..."
sudo tee /etc/systemd/system/myvpn.service > /dev/null <<EOF
[Unit]
Description=MyVPN Server (AES-256-GCM Encrypted VPN)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/myvpn-server -iface $IFACE
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

# 5. Servisi aktifleştir ve başlat
sudo systemctl daemon-reload
sudo systemctl enable myvpn
sudo systemctl start myvpn

echo ""
echo "═══════════════════════════════════"
echo "  ✓ MyVPN Server KURULDU!"
echo "═══════════════════════════════════"
echo ""
echo "  Durum:     sudo systemctl status myvpn"
echo "  Loglar:    sudo journalctl -u myvpn -f"
echo "  Durdur:    sudo systemctl stop myvpn"
echo "  Yeniden:   sudo systemctl restart myvpn"
echo ""
echo "  Server artık 7/24 çalışıyor."
echo "  Makine restart olsa bile otomatik açılır."
echo ""
