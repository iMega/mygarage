REPO = github.com/imega/mygarage
CWD = /go/src/$(REPO)

GIT_SHA ?= $(shell git rev-parse --verify -q HEAD || echo dev)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -s -w \
	-X $(REPO)/version.SHA=$(GIT_SHA) \
	-X $(REPO)/version.BuildTime=$(BUILD_TIME)

LINUX_BIN = dist/mygarage-linux-amd64

.PHONY: build build-linux run clean install tidy test dist-mac clean-dist

build:
	go build -ldflags "$(LDFLAGS)" -o mygarage ./cmd/main.go

build-linux:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
		-ldflags "$(LDFLAGS)" -o $(LINUX_BIN) ./cmd/main.go
	cd dist && sha256sum $(notdir $(LINUX_BIN)) > $(notdir $(LINUX_BIN)).sha256

run: build
	./mygarage

clean:
	rm -f mygarage
	rm -f *.db *.db-shm *.db-wal
	rm -rf backups/

clean-dist:
	rm -rf dist

dist-mac: clean-dist
	@echo "Building macOS app bundle..."
	mkdir -p dist/myGarage.app/Contents/MacOS
	mkdir -p dist/myGarage.app/Contents/Resources
	go build -ldflags "$(LDFLAGS)" -o dist/myGarage.app/Contents/MacOS/mygarage-server ./cmd/main.go
	swiftc -framework Cocoa -o dist/myGarage.app/Contents/MacOS/mygarage macos/Launcher.swift
	chmod +x dist/myGarage.app/Contents/MacOS/mygarage
	chmod +x dist/myGarage.app/Contents/MacOS/mygarage-server
	@echo "Creating app icon..."
	mkdir -p dist/AppIcon.iconset
	sips -z 16 16 mygarage.png --out dist/AppIcon.iconset/icon_16x16.png
	sips -z 32 32 mygarage.png --out dist/AppIcon.iconset/icon_16x16@2x.png
	sips -z 32 32 mygarage.png --out dist/AppIcon.iconset/icon_32x32.png
	sips -z 64 64 mygarage.png --out dist/AppIcon.iconset/icon_32x32@2x.png
	sips -z 128 128 mygarage.png --out dist/AppIcon.iconset/icon_128x128.png
	sips -z 256 256 mygarage.png --out dist/AppIcon.iconset/icon_128x128@2x.png
	sips -z 256 256 mygarage.png --out dist/AppIcon.iconset/icon_256x256.png
	sips -z 512 512 mygarage.png --out dist/AppIcon.iconset/icon_256x256@2x.png
	sips -z 512 512 mygarage.png --out dist/AppIcon.iconset/icon_512x512.png
	sips -z 1024 1024 mygarage.png --out dist/AppIcon.iconset/icon_512x512@2x.png
	iconutil -c icns dist/AppIcon.iconset -o dist/myGarage.app/Contents/Resources/AppIcon.icns
	@echo "Creating Info.plist..."
	@echo '<?xml version="1.0" encoding="UTF-8"?>' > dist/myGarage.app/Contents/Info.plist
	@echo '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' >> dist/myGarage.app/Contents/Info.plist
	@echo '<plist version="1.0">' >> dist/myGarage.app/Contents/Info.plist
	@echo '<dict>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<key>CFBundleExecutable</key>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<string>mygarage</string>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<key>CFBundleIdentifier</key>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<string>com.github.imega.mygarage</string>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<key>CFBundleName</key>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<string>myGarage</string>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<key>CFBundleVersion</key>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<string>1.0.0</string>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<key>CFBundleShortVersionString</key>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<string>1.0.0</string>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<key>CFBundlePackageType</key>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<string>APPL</string>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<key>CFBundleIconFile</key>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<string>AppIcon</string>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<key>LSMinimumSystemVersion</key>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<string>10.13</string>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<key>NSHighResolutionCapable</key>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<true/>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<key>LSUIElement</key>' >> dist/myGarage.app/Contents/Info.plist
	@echo '	<true/>' >> dist/myGarage.app/Contents/Info.plist
	@echo '</dict>' >> dist/myGarage.app/Contents/Info.plist
	@echo '</plist>' >> dist/myGarage.app/Contents/Info.plist
	@echo "macOS app bundle created at dist/myGarage.app"

install:
	go mod download

tidy:
	go mod tidy

test:
	go test -v ./...

lint:
	@-docker run --rm -t -v $(CURDIR):$(CWD) -w $(CWD) \
		golangci/golangci-lint golangci-lint run

help:
	@echo "Available targets:"
	@echo "  build      - Build the application"
	@echo "  build-linux - Build static linux/amd64 server binary + sha256"
	@echo "  run        - Build and run the application"
	@echo "  clean      - Remove built files and database"
	@echo "  clean-dist - Remove dist directory"
	@echo "  dist-mac   - Build macOS app bundle in dist/"
	@echo "  install    - Download dependencies"
	@echo "  tidy       - Tidy go.mod"
	@echo "  test       - Run tests"
