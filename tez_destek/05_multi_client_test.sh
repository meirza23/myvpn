#!/usr/bin/env bash
# Şekil 5.3 / Tablo 5.3 — Çok istemcili bağlantı testi
#
# Sunucuyu başlat:
#   sudo ./bin/myvpn-server -iface eth0 2>&1 | tee server.log
#
# Sonra iki ayrı terminalden iki istemci bağlat:
#   sudo ./bin/myvpn-cli  (terminal 1)
#   sudo ./bin/myvpn-cli  (terminal 2)
#
# Beklenen sunucu log satırları:
#   [Handshake] Yeni oturum → 10.0.0.2
#   [Handshake] Yeni oturum → 10.0.0.3
#
# Bu logları kaydet → Şekil 5.3
echo "Bu çağrı yalnızca rehber içindir; lütfen yukarıdaki adımları sırayla uygula."
