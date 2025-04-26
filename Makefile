# Makefile

# Variables
TARGET_NAME := codecat
INSTALL_DIR := $(HOME)/.local/bin
INSTALL_PATH := $(INSTALL_DIR)/$(TARGET_NAME)
SRC_DIR := ./cmd/codecat
GO_FILES := $(wildcard $(SRC_DIR)/*.go)

# Default target (optional, can just run 'make local-bin')
default: local-bin

# Target to build and install the binary to ~/.local/bin
# It depends on the Go source files. Make will check if any Go file
# is newer than the installed binary.
$(INSTALL_PATH): $(GO_FILES)
	@echo "Building $(TARGET_NAME) from $(SRC_DIR)..."
	@go build -ldflags="-X main.Version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)" -o $(TARGET_NAME) $(SRC_DIR)
	@echo "Installing $(TARGET_NAME) to $(INSTALL_DIR)..."
	@mkdir -p $(INSTALL_DIR)
	@install $(TARGET_NAME) $(INSTALL_PATH)
	@rm $(TARGET_NAME)
	@echo "$(TARGET_NAME) installed successfully to $(INSTALL_PATH)"

# Rule to explicitly build and install
# Use .PHONY to ensure it always runs when invoked directly
# and to indicate it's an action, not necessarily a file target name.
.PHONY: local-bin
local-bin: $(INSTALL_PATH)

# Clean target (optional)
.PHONY: clean
clean:
	@echo "Cleaning..."
	@rm -f $(TARGET_NAME)