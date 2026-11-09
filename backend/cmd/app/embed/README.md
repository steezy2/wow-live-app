# Built-in files

`build.sh` copies the built page (`dist/`), `mplus.toml` and `mplus-dungeons.json` here for `go:embed`, and
`cmd/appicon` writes `icon.ico`; they are gitignored. This file keeps the folder in git so `go vet ./...` works on a fresh Windows checkout.
