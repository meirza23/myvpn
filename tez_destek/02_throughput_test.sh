#!/usr/bin/env bash
# Tablo 5.2 — Bant Genişliği (Throughput) Ölçüm Testi
#
# Önkoşul: iperf3 hem istemcide hem de bir hedef sunucuda kurulu.
#   sudo apt install iperf3
#
# 1) Hedef makinede çalıştır:  iperf3 -s
# 2) İstemcide bu betiği çalıştır (önce VPN kapalı, sonra açık):
#       ./02_throughput_test.sh <hedef_ip>
set -e
TARGET="${1:?Kullanım: $0 <hedef_ip>}"
echo "=== TCP throughput → $TARGET ==="
iperf3 -c "$TARGET" -t 30 | tail -3
echo
echo "=== UDP throughput → $TARGET (200 Mbps deneme) ==="
iperf3 -c "$TARGET" -u -b 200M -t 30 | tail -5
