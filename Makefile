# The @ before a command in a Makefile suppresses the command itself from being echoed to the terminal.
# Target to run when no target is specified

.DEFAULT_GOAL := test

.PHONY: fmt vet test clean lint integration fuzz

TEST_FLAGS ?=
FUZZTIME ?= 30s

fmt:
	go fmt ./...

vet: 
	go vet ./...

test: 
	go test $(TEST_FLAGS) ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out
	
clean: 
	go clean

lint:
	golangci-lint run

integration:
	cd integration && make test

fuzz:
	go test -run '^$$' -fuzz '^FuzzLex$$' -fuzztime $(FUZZTIME) .
	go test -run '^$$' -fuzz '^FuzzWritePlaceholders$$' -fuzztime $(FUZZTIME) .
