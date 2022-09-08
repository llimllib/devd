source = $(shell find . -iname '*.go' -type f)

.PHONY: install
install: $(source)
	go install ./cmd/devd

.PHONY: test
test:
	go test ./...

.PHONY: check
check:
	@# install required tools if not found. Should pin versions if the latest
	@# version ever starts to cause problems.
	@command -v staticcheck 1>/dev/null || \
		go install honnef.co/go/tools/cmd/staticcheck@latest
	@command -v govulncheck 1>/dev/null || \
		go install golang.org/x/vuln/cmd/govulncheck@latest
	@# they recommend installing from your package manager instead, but this has
	@# a hope of working cross-platform, so I'll put it here instead
	@# https://golangci-lint.run/usage/install/
	@command -v golangci-lint 1>/dev/null || \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.49.0

	staticcheck ./...
	govulncheck ./...
	golangci-lint run
