.DEFAULT_GOAL := test

.PHONY: fmt vet test lint fuzz bench clean oracle oracle-corpus oracle-fuzz oracle-lint corpus

PKGS ?= . ./internal/...
TEST_FLAGS ?=
FUZZTIME ?= 30s
PG_TAG ?= REL_17_7

fmt:
	go fmt $(PKGS)

vet:
	go vet $(PKGS)

test:
	go test $(TEST_FLAGS) $(PKGS) -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

lint:
	golangci-lint run $(PKGS)

fuzz:
	go test -run '^$$' -fuzz '^FuzzParse$$' -fuzztime $(FUZZTIME) ./internal/syntax
	go test -run '^$$' -fuzz '^FuzzLex$$' -fuzztime $(FUZZTIME) ./internal/syntax
	go test -run '^$$' -fuzz '^FuzzRender$$' -fuzztime $(FUZZTIME) ./internal/render
	go test -run '^$$' -fuzz '^FuzzCompose$$' -fuzztime $(FUZZTIME) ./internal/render

bench:
	go test -run '^$$' -bench . -benchmem $(PKGS)

clean:
	go clean
	rm -f coverage.out

# The oracle checks glimt against Postgres's own parser, and against a live
# Postgres when GLIMT_PG_DSN is set. It lives in the integration module.
oracle:
	cd integration && go test $(TEST_FLAGS) ./...

# Fetch Postgres's regression suite, the largest part of the oracle corpus.
oracle-corpus:
	cd integration && go run ./cmd/fetchregress $(PG_TAG) testdata/regress

oracle-fuzz:
	cd integration && go test -run '^$$' -fuzz '^FuzzOracle$$' -fuzztime $(FUZZTIME) .

oracle-lint:
	cd integration && golangci-lint run ./...

# Regenerate the oracle corpus of SQL from glimt's own tests.
corpus:
	cd integration && go run ./cmd/gencorpus ../internal > testdata/glimt.jsonl
