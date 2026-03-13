# The @ before a command in a Makefile suppresses the command itself from being echoed to the terminal.
# Target to run when no target is specified

.DEFAULT_GOAL := test

.PHONY: fmt vet test test-local clean

TEST_FLAGS ?=

fmt:
	go fmt ./...

test: 
	go test $(TEST_FLAGS) ./... 

clean: 
	go clean

