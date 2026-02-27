BIN := gh-report
CONFIG := config.local.yaml

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-X github.com/miclle/gh-report/cmd.Version=$(VERSION) \
                     -X github.com/miclle/gh-report/cmd.Commit=$(COMMIT) \
                     -X github.com/miclle/gh-report/cmd.BuildDate=$(DATE)"

.PHONY: run build clean install

run: build
	./$(BIN) --config $(CONFIG)

build:
	go build $(LDFLAGS) -o $(BIN) .

install:
	go install $(LDFLAGS) .

clean:
	rm -f $(BIN)
