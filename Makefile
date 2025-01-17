
default: build

.PHONY: clean
clean:
	rm -rf build/bin || true
	rm -rf frontend/dist || true

.PHONY: wails
wails:
	(which wails >/dev/null && wails update) || go install github.com/wailsapp/wails/v2/cmd/wails@latest

.PHONY: build
build: clean wails
	wails build -ldflags="-X 'github.com/ghostsecurity/reaper/version.Date=$$(date)'"

.PHONY: test
test: test-go test-js

.PHONY: test-go
test-go:
	go clean -testcache
	go test ./... -race

.PHONY: test-js
test-js:
	cd frontend && npm install && npm test

.PHONY: lint
lint: lint-go lint-js

.PHONY: lint-js
lint-js:
	cd frontend && npm install && npm run lint

.PHONY: lint-go
lint-go:
	which golangci-lint || go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.50.1
	golangci-lint run --timeout 3m --verbose

.PHONY: run
run: clean wails
	REAPER_LOG_LEVEL=debug wails dev -ldflags="-X 'github.com/ghostsecurity/reaper/version.Date=$$(date)'"

.PHONY: fix
fix:
	cd frontend && npm install && npm run fix

.PHONY: docs
docs:
	cd docs && bundle install && bundle exec jekyll serve --livereload

.PHONY: install
install: build
	@./scripts/install.sh

