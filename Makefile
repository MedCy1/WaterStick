.PHONY: all build test clean run-cli run-tui run-gui

BIN_DIR := bin

all: build

build:
	@mkdir -p $(BIN_DIR)
	@echo "Building WaterStick CLI (cmd/waterstick)..."
	@go build -o $(BIN_DIR)/waterstick ./cmd/waterstick
	@echo "Building WaterStick TUI (cmd/waterstick-tui with Bubble Tea v2)..."
	@go build -o $(BIN_DIR)/waterstick-tui ./cmd/waterstick-tui
	@echo "Building WaterStick GUI (cmd/waterstick-gui with Wails v2)..."
	@go build -tags "production,webkit2_41" -o $(BIN_DIR)/waterstick-gui ./cmd/waterstick-gui
	@echo "All WaterStick binaries built in $(BIN_DIR)/"

test:
	@echo "Running unit tests & security checks..."
	@go test -v ./pkg/waterstick/...

clean:
	@rm -rf $(BIN_DIR)
	@echo "Clean complete."

run-cli:
	@./$(BIN_DIR)/waterstick --help

run-tui:
	@./$(BIN_DIR)/waterstick-tui

run-gui:
	@./$(BIN_DIR)/waterstick-gui
