source = $(shell find . -iname '*.go' -type f)

.PHONY: install
install: $(source)
	go install ./cmd/devd

.PHONY: test
test:
	go test ./...

.PHONY: check
check:
	# install required tools if not found. Should pin versions if the latest
	# version ever starts to cause problems.
	command -v staticcheck || go install honnef.co/go/tools/cmd/staticcheck@latest
	command -v govulncheck || go install golang.org/x/vuln/cmd/govulncheck@latest

	staticcheck ./...
	govulncheck ./...
