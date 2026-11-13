# WoW Live's everyday commands, from the repo root in Git Bash (with Go, Node and make): `make` lists them.
SHELL := bash
.DEFAULT_GOAL := help
.PHONY: help app page site check fmt swag ref ref-check deps release

help: ## List these commands
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | sed -E 's/:.*## /\t/' | expand -t 12

app: ## Build bin/WoW Live.exe (the page, the icon, then the app)
	bash backend/cmd/app/build.sh

deps:
	cd frontend && { test -d node_modules || npm ci; }

page: deps ## Build the page into frontend/dist
	cd frontend && npm run build

site: page ## Serve the page and API in a browser at http://localhost:8080
	cd backend && go run ./cmd/site

check: deps ## Run what CI runs: gofmt, go vet and go test, then eslint, vitest and the page's build
	cd backend && test -z "$$(gofmt -l . | tee /dev/stderr)" && go vet ./... && go test ./...
	cd frontend && npm run lint && npm test && npm run build

fmt: ## gofmt the backend
	cd backend && gofmt -w .

swag: ## Regenerate the API spec (backend/docs) after changing an endpoint or its types
	cd backend && swag init -g cmd/site/main.go -o docs --parseInternal

ref: ## Rewrite reference/mplus-dungeons.json from its sources: check the diff
	cd backend && go run ./cmd/mplusref

ref-check: ## List what the logged keys show that the dungeon file lacks
	cd backend && go run ./cmd/mplusref --check

release: ## Tag main and push the tag, which builds the release (asks first; suggests the next patch)
	@set -e; \
	read -r -p "Are you sure you want to release now? (y/n): " ok; [ "$$ok" = y ] || { echo Cancelled; exit 1; }; \
	[ "$$(git branch --show-current)" = main ] || { echo "Switch to main first"; exit 1; }; \
	git fetch -q origin main --tags; \
	[ -z "$$(git status --porcelain)" ] && [ "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/main)" ] || { echo "main must be clean and match origin/main"; exit 1; }; \
	last=$$(git tag --list 'v*' --sort=-v:refname | head -1); \
	next=$$(echo "$${last:-v0.0.0}" | awk -F. '{print $$1"."$$2"."$$3+1}'); \
	read -r -p "Tag (latest is $${last:-none}) [$$next]: " tag; tag=$${tag:-$$next}; \
	case "$$tag" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo "Tag must look like v1.2.3"; exit 1;; esac; \
	git rev-parse -q --verify "refs/tags/$$tag" >/dev/null && { echo "$$tag already exists"; exit 1; }; \
	grep -q "^## \[$${tag#v}\]" CHANGELOG.md || echo "Warning: CHANGELOG.md has no $${tag#v} heading"; \
	read -r -p "Release $$tag now? (y/n): " ok; [ "$$ok" = y ] || { echo Cancelled; exit 1; }; \
	git tag "$$tag" && git push origin "$$tag" && echo "Released $$tag"
