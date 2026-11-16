# Built-in files

`build.sh` copies the built page (`dist/`), the rules (`*.toml`) and `reference/` here, laid out as in the repo, for
`go:embed`, and `cmd/appicon` writes `icon.ico`; they are gitignored. This file keeps the folder in git so
`go vet ./...` works on a fresh Windows checkout.
