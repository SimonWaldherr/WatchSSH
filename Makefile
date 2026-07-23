BIN ?= watchssh
CONFIG ?= config.yaml

.PHONY: help fmt test vet build check run once clean

help:
	@printf '%s\n' 'WatchSSH development targets:'
	@printf '%s\n' '  make fmt                 Format Go sources'
	@printf '%s\n' '  make test                Run the test suite'
	@printf '%s\n' '  make vet                 Run go vet'
	@printf '%s\n' '  make build               Build ./$(BIN)'
	@printf '%s\n' '  make check               Run fmt, test, vet, and build'
	@printf '%s\n' '  make run CONFIG=...      Start WatchSSH'
	@printf '%s\n' '  make once CONFIG=...     Run one collection cycle'
	@printf '%s\n' '  make clean               Remove the local binary'

fmt:
	gofmt -w $$(rg --files -g '*.go' -g '!vendor/**')

test:
	go test ./...

vet:
	go vet ./...

build:
	go build -o $(BIN) .

check: fmt test vet build

run:
	go run . -config $(CONFIG)

once:
	go run . -config $(CONFIG) -once

clean:
	rm -f $(BIN)
