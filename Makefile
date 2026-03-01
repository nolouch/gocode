GO ?= go
APP := gocode
CMD := ./cmd/gocode
BIN_DIR := bin
BIN := $(BIN_DIR)/$(APP)

.PHONY: help build run tui serve test fmt vet clean

help:
	@echo "Targets:"
	@echo "  make build              Build $(BIN)"
	@echo "  make run PROMPT='...'   Run one-shot prompt"
	@echo "  make tui                Start interactive TUI"
	@echo "  make serve              Start headless server"
	@echo "  make test               Run all tests"
	@echo "  make fmt                Format Go packages"
	@echo "  make vet                Run go vet"
	@echo "  make clean              Remove built binary"

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN) $(CMD)

run:
	@test -n "$(PROMPT)" || (echo "Usage: make run PROMPT='analyze the repo'" && exit 1)
	$(GO) run $(CMD) run -p "$(PROMPT)"

tui:
	$(GO) run $(CMD) tui

serve:
	$(GO) run $(CMD) serve

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

clean:
	@rm -f $(BIN)
