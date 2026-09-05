.PHONY: fmt fmt-check lint tidy-check test golden check release-check

# Auto-format the code (gofumpt + goimports via golangci-lint).
fmt:
	golangci-lint fmt ./...

# Fail if the code is not formatted (mirrors the CI format gate).
fmt-check:
	golangci-lint fmt --diff ./...

# Run the linters (mirrors the CI lint step).
lint:
	golangci-lint run ./...

# Fail if go.mod/go.sum are not tidy (mirrors the CI module-hygiene gate).
tidy-check:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

# Run tests with the race detector and coverage (mirrors the CI test step).
test:
	gotestsum --format testdox -- -race -coverprofile=coverage.out -covermode=atomic ./...

# Re-record the golden files of every view. Read the diff before committing
# it: seeing what changed on the screen is the point of the recordings.
golden:
	OCTOSCOPE_UPDATE_GOLDEN=1 go test ./...

# Run everything the CI checks, locally.
check: tidy-check lint fmt-check test

# Validate the release configuration and cross-compilation (mirrors release CI).
release-check:
	goreleaser check
	GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/octoscope
	GOOS=darwin GOARCH=arm64 go build -o /dev/null ./cmd/octoscope
	GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/octoscope
