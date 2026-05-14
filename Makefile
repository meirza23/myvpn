BINARY_DIR := bin
SERVER_BIN := $(BINARY_DIR)/myvpn-server
GUI_BIN    := $(BINARY_DIR)/myvpn-gui
CLI_BIN    := $(BINARY_DIR)/myvpn-cli

.PHONY: all build-server build-gui build-cli run-server run-gui run-cli clean

all: build-all

build-all: build-server build-gui build-cli
	@echo "✓ Tüm binary'ler derlendi: $(BINARY_DIR)/"

build-server:
	@mkdir -p $(BINARY_DIR)
	go build -buildvcs=false -o $(SERVER_BIN) ./cmd/server/
	@echo "✓ $(SERVER_BIN)"

build-gui:
	@mkdir -p $(BINARY_DIR)
	go build -buildvcs=false -o $(GUI_BIN) ./cmd/gui/
	@echo "✓ $(GUI_BIN)"

build-cli:
	@mkdir -p $(BINARY_DIR)
	go build -buildvcs=false -o $(CLI_BIN) ./cmd/client/
	@echo "✓ $(CLI_BIN)"

run-server: build-server
	@echo "Sunucu başlatılıyor (sudo gerekli)..."
	sudo $(SERVER_BIN) -iface eth0

run-gui: build-gui
	@echo "GUI istemcisi başlatılıyor (sudo gerekli)..."
	sudo $(GUI_BIN)

run-cli: build-cli
	@echo "CLI istemcisi başlatılıyor (sudo gerekli)..."
	sudo $(CLI_BIN)

install-server: build-server
	@echo "Sunucu kurulum scripti çalıştırılıyor..."
	bash scripts/install-server.sh

clean:
	rm -rf $(BINARY_DIR)
	@echo "✓ $(BINARY_DIR)/ temizlendi."
