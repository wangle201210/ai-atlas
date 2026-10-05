# AI Atlas

- Stack: Wails v3.0.0-beta.27, Vue 3 + TypeScript, Go 1.25+, SQLite (mattn/go-sqlite3, CGO).
- UI and product copy are Chinese. Keep all indexing local and preserve explicit uncertainty in temporary-file attribution.
- Never test cleanup against the user's real Codex home or real temporary files. Use t.TempDir and .test-data fixtures with a fake Codex CLI.
- Store Atlas state separately from Codex. Do not edit Codex's SQLite/index files directly.
- Every destructive action requires a concrete preview, path checks, freshness checks, occupancy checks and explicit in-app confirmation. Preserve token history before removing source logs.
- Go core checks: go test -race ./internal/... ./cmd/atlas-index; go vet ./...
- Build/bindings: wails3 build. Do not hand-edit frontend/bindings.
- E2E: python3 scripts/create-fixture.py; go build -tags server -o bin/ai-atlas-server .; cd frontend && npm run test:e2e.

- Self-updates target only wangle201210/ai-atlas Releases. Require SHA256SUMS and matching app-bundle assets; never exercise restart/replacement against the real installed app in automated tests.
- Release version source: internal/buildinfo/VERSION. Use scripts/version.py to synchronize metadata; scripts/package-release.py creates local artifacts; only pushing a v* tag publishes a Release.
