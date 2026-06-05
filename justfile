default: build

# Build the project
build:
	go build -o vega-cli ./cmd/vega-cli

# Run the project
run: build
	./vega-cli

# Run tests
test:
	go test -v ./...

# Format and lint code
lint:
	go fmt ./...
	go vet ./...

# Create a clean release build using GoReleaser (requires goreleaser installed)
release:
	goreleaser release --snapshot --clean

# Tidy Go modules
tidy:
	go mod tidy