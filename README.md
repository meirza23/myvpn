# ⬡ MyVPN

AES-256-GCM şifreli, TUN tabanlı bir VPN uygulaması. Go dilinde yazılmış olup Fyne GUI istemcisi ve terminal CLI istemcisi içerir.

---

## Mimari

```
┌─────────────────────────────────────────────────────────┐
│                    İstemci (Linux)                       │
│  ┌──────────────┐   ┌──────────────┐                    │
│  │  GUI (Fyne)  │   │  CLI (term)  │                    │
│  └──────┬───────┘   └──────┬───────┘                    │
│         └─────────┬─────────┘                           │
│              TUN arayüzü (10.0.0.x)                     │
│         AES-256-GCM şifreli UDP tüneli                  │
└──────────────────────────┬──────────────────────────────┘
                           │
            ┌──────────────▼──────────────────┐
            │         Sunucu (Linux/VPS)       │
            │   Port 8079 → Handshake         │
            │   Port 8080 → TCP taşıyıcı      │
            │   Port 8081 → UDP taşıyıcı      │
            │   TUN (10.0.0.1) + NAT          │
            └─────────────────────────────────┘
```

---

## Özellikler

- 🔒 **AES-256-GCM** şifreleme (her paket için benzersiz nonce)
- 👥 **Çoklu istemci** — her bağlanan istemciye otomatik sanal IP atanır (10.0.0.2–10.0.0.254)
- 🖥️ **Fyne GUI** — Bağlan + Ayarlar sekmeleri, canlı trafik istatistikleri
- ⌨️ **CLI istemcisi** — terminal üzerinden kullanım
- ⚙️ **JSON konfigürasyon** — `~/.myvpn/client.json` ve `~/.myvpn/server.json`
- 🔌 **Handshake protokolü** — istemci bağlanınca sanal IP otomatik atanır

---

## Gereksinimler

- Linux (Ubuntu 20.04+ önerilir)
- Go 1.21+
- `iproute2`, `iptables`
- TUN/TAP sürücüsü (`/dev/net/tun`)

---

## Kurulum

### Bağımlılıkları Yükle

```bash
go mod download
```

### Derleme

```bash
make build-all
# Çıktı: bin/myvpn-server, bin/myvpn-gui, bin/myvpn-cli
```

---

## Sunucu Kurulumu (VPS)

```bash
# Projeyi sunucuya kopyala
scp -r . user@sunucu-ip:/opt/myvpn

# Sunucuda çalıştır
cd /opt/myvpn
bash scripts/install-server.sh
```

Veya manuel:

```bash
make build-server
sudo ./bin/myvpn-server -iface eth0
```

Sunucu, `~/.myvpn/server.json` dosyasından ayarları okur. İlk çalıştırmada otomatik oluşturulur.

---

## İstemci Kullanımı

### GUI

```bash
make run-gui
# veya
sudo ./bin/myvpn-gui
```

1. **Ayarlar** sekmesinden sunucu IP, port ve AES anahtarını girin → **Kaydet**
2. **Bağlan** sekmesinden **Bağlan** butonuna tıklayın

### CLI

```bash
make run-cli
# veya
sudo ./bin/myvpn-cli
```

Menüden `1` seçerek bağlanın. Sunucu IP için Enter'a basarsanız `~/.myvpn/client.json`'dan okunur.

---

## Konfigürasyon

### `~/.myvpn/client.json`

```json
{
  "server_ip": "1.2.3.4",
  "port": 8079,
  "vpn_key": "12345678901234567890123456789012"
}
```

### `~/.myvpn/server.json`

```json
{
  "port": 8079,
  "vpn_key": "12345678901234567890123456789012",
  "out_iface": "eth0"
}
```

> ⚠️ `vpn_key` istemci ve sunucuda **aynı** olmalı ve **tam 32 karakter** olmalıdır.

---

## Güvenlik Notu

- VPN anahtarını güvenli bir kanaldan paylaşın (örn: SSH üzerinden).
- Anahtarı git'e göndermeyin. `.gitignore`'a `~/.myvpn/` eklemeyi unutmayın.

---

## Lisans

MIT
