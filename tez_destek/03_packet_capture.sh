#!/usr/bin/env bash
# Şekil 5.2 — tcpdump ile şifreli paket yakalama
# 1) Bu betiği VPN bağlıyken sunucunun dış arayüzünde çalıştır
# 2) Aynı anda istemciden ping/curl yap
# 3) Çıktıyı ekran görüntüsü olarak al → Şekil 5.2
IFACE="${1:-eth0}"
PORT="${2:-8081}"
echo "[+] $IFACE üzerinde UDP $PORT yakalanıyor (Ctrl-C ile durdur)…"
sudo tcpdump -i "$IFACE" -nn -X -c 10 "udp port $PORT"
