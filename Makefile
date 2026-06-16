APP_NAME := dotndoh-checker
VERSION := $(strip $(shell powershell -NoProfile -Command "(Get-Content VERSION -Raw).Trim()"))
DIST_DIR := dist

ifeq ($(OS),Windows_NT)
	BINARY := $(APP_NAME).exe
	GOOS_VALUE := windows
	GOARCH_VALUE := amd64
else
	BINARY := $(APP_NAME)
	GOOS_VALUE := $(shell go env GOOS)
	GOARCH_VALUE := $(shell go env GOARCH)
endif

PACKAGE_NAME := $(APP_NAME)-v$(VERSION)-$(GOOS_VALUE)-$(GOARCH_VALUE)
PACKAGE_DIR := $(DIST_DIR)/$(PACKAGE_NAME)
ARCHIVE := $(DIST_DIR)/$(PACKAGE_NAME).zip

.PHONY: help run binary package build clean

help:
	@echo "Targets:"
	@echo "  make run      Run checker from source"
	@echo "  make binary   Build binary into $(DIST_DIR)"
	@echo "  make package  Build binary and create zip package"
	@echo "  make build    Alias for package"
	@echo "  make clean    Remove build artifacts"
	@echo "Version: $(VERSION)"

run:
	go run .

binary:
	powershell -NoProfile -ExecutionPolicy Bypass -Command "New-Item -ItemType Directory -Force '$(DIST_DIR)' | Out-Null"
	go build -trimpath -ldflags="-s -w -X main.appVersion=$(VERSION)" -o "$(DIST_DIR)/$(BINARY)" .

package: binary
	powershell -NoProfile -ExecutionPolicy Bypass -Command "Remove-Item -Recurse -Force '$(PACKAGE_DIR)' -ErrorAction SilentlyContinue; New-Item -ItemType Directory -Force '$(PACKAGE_DIR)' | Out-Null; Copy-Item '$(DIST_DIR)/$(BINARY)' '$(PACKAGE_DIR)/'; Copy-Item 'DoT.txt','DoH.txt','README.md','LICENSE','VERSION' '$(PACKAGE_DIR)/'"
	powershell -NoProfile -ExecutionPolicy Bypass -Command "Compress-Archive -Path '$(PACKAGE_DIR)/*' -DestinationPath '$(ARCHIVE)' -Force"
	@echo "Built $(ARCHIVE)"

build: package

clean:
	powershell -NoProfile -ExecutionPolicy Bypass -Command "Remove-Item -Recurse -Force '$(DIST_DIR)' -ErrorAction SilentlyContinue"
