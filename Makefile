BIN := gh-report
CONFIG := config.local.yaml

.PHONY: run build clean

run: build
	./$(BIN) -config $(CONFIG)

build:
	go build -o $(BIN) .

clean:
	rm -f $(BIN)
