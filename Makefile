.DEFAULT_GOAL = build

BIN_NAME ?= ragx

VERSION ?= 0.0.0

# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_VERSION ?= v2.6.0
TEST_ARGS = -v -timeout 40s -coverpkg=./...

PKG_PATH ?= main
LDFLAGS = -X '$(PKG_PATH).Version=$(VERSION)'

bin/golangci-lint-$(GOLANGCI_VERSION):
	@mkdir -p bin
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
    	| sh -s -- -b ./bin $(GOLANGCI_VERSION)
	@mv bin/golangci-lint "$@"

bin/golangci-lint: bin/golangci-lint-$(GOLANGCI_VERSION)
	@ln -sf golangci-lint-$(GOLANGCI_VERSION) bin/golangci-lint

bin/$(BIN_NAME): go-mod-tidy
	go build -ldflags "$(LDFLAGS)" -o bin/$(BIN_NAME) ./cmd/$(BIN_NAME)

.PHONY: build
build: bin/$(BIN_NAME)

.PHONY: build-dist
build-dist: build
	mkdir -p dist
	cp ./bin/$(BIN_NAME) UNLICENSE ./dist/

.PHONY: go-mod-tidy
go-mod-tidy:
	go mod tidy

.PHONY: clean
clean:
	go clean -testcache
	rm -rf coverage/ bin/ dist/

.PHONY: test
test:
	go test $(TEST_ARGS) ./...

.PHONY: cover
cover:
	@mkdir -p coverage
	go test $(TEST_ARGS) ./... -coverprofile coverage/cover.out ./...
	@go tool cover -func=./coverage/cover.out | grep total | awk '{print "total coverage: " $$3}'

.PHONY: coverage-html
coverage-html: cover
	go tool cover -html=coverage/cover.out -o coverage/index.html

.PHONY: lint
lint: bin/golangci-lint
	bin/golangci-lint run

.PHONY: fix
fix: bin/golangci-lint
	bin/golangci-lint run --fix

.PHONY: check
check: lint test

.PHONY: assets
assets: build
	./bin/$(BIN_NAME) config generate > assets/default-config.toml
	./bin/$(BIN_NAME) > assets/usage.txt

.PHONY: readme.md
readme.md: assets readme.templ.md assets/default-config.toml assets/usage.txt
	./scripts/readme_gen.sh readme.templ.md readme.md