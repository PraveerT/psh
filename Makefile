.PHONY: cli android install clean

# ── CLI ──────────────────────────────────────────────────────────────────────

cli:
	cd cli && go mod tidy && go build -o ../bin/psh .

cli-all:
	cd cli && go mod tidy
	mkdir -p bin
	GOOS=darwin  GOARCH=arm64  go build -C cli -o ../bin/psh-darwin-arm64  .
	GOOS=darwin  GOARCH=amd64  go build -C cli -o ../bin/psh-darwin-amd64  .
	GOOS=linux   GOARCH=amd64  go build -C cli -o ../bin/psh-linux-amd64   .
	GOOS=windows GOARCH=amd64  go build -C cli -o ../bin/psh-windows.exe   .

install: cli
	cp bin/psh /usr/local/bin/psh
	@echo "Installed psh to /usr/local/bin/psh"

# ── Android ──────────────────────────────────────────────────────────────────

android:
	cd android && ./gradlew assembleDebug

android-release:
	cd android && ./gradlew assembleRelease

android-install:
	cd android && ./gradlew installDebug

# ── Dev helpers ──────────────────────────────────────────────────────────────

clean:
	rm -rf bin/
	cd android && ./gradlew clean
