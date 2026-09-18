.PHONY: test lint build bench fmt vet docs eval verify fuzz
PYTHON ?= python3
test:
	go test -race -count=1 ./...
lint:
	golangci-lint run ./...
build:
	go build -trimpath -o bin/compactd ./cmd/compactd
bench:
	go test ./server -run '^$$' -bench BenchmarkHTTPReplay -benchmem -count=6 -benchtime=200ms
fmt:
	gofmt -w compact archive token jev server cmd
vet:
	go vet ./...
docs:
	$(PYTHON) scripts/check_repo.py
eval: build
	$(PYTHON) eval/run.py --binary ./bin/compactd --replay
verify:
	$(PYTHON) scripts/verify.py
fuzz:
	go test ./compact -run '^$$' -fuzz '^FuzzExcerpt$$' -fuzztime=5s
	go test ./compact -run '^$$' -fuzz '^FuzzCompactionInvariants$$' -fuzztime=5s
