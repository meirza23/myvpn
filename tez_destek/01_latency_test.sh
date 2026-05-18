#!/usr/bin/env bash
# Şekil/Tablo 5.1 — Gecikme (Latency) Ölçüm Testi
#
# Bu betiği iki kez çalıştır:
#   1) VPN KAPALI iken
#   2) VPN AÇIK iken (sudo ./bin/myvpn-cli ile bağlandıktan sonra)
#
# Sonuçları Tablo 5.1'e yaz.
set -e
TARGET="${1:-8.8.8.8}"   # default hedef: Google DNS
COUNT=50
echo "=== Latency test → $TARGET ($COUNT paket) ==="
ping -c "$COUNT" -i 0.2 "$TARGET" | tail -2
echo "Yukarıdaki çıktıdaki min/avg/max/mdev değerlerini Tablo 5.1'e yaz."
