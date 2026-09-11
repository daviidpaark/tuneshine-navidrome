# TuneShine Navidrome Instructions

## Repository Role & Architecture
- Navidrome music server plugin for TuneShine.
- Written in Go (Go 1.24+), compiled to WebAssembly (`plugin.wasm`), packaged into a `.ndp` archive (`tuneshine.ndp`) with `manifest.json`.
- Listens to Navidrome scrobble/playback events and reports real-time state and metadata to TuneShine Hub.

## Development & Validation
- Run unit tests from module root:
  ```bash
  go test ./...
  ```
- Build and package plugin:
  ```bash
  make package
  ```
- All automated checks and tests should pass before concluding changes.

## Releases & CI
- Releases publish `.ndp` plugin packages (`tuneshine.ndp`) via `.github/workflows/publish-plugin.yml`.
- **Packaging Format:** Prefer `.ndp` plugin packaging. Do not generate or keep `.exe` artifacts for plugin releases.
- Triggered by semantic version tags `v*.*.*` (e.g. `v0.1.0`) or manual `workflow_dispatch` with a `tag` input.
- Always keep `CHANGELOG.md` and `manifest.json` version declarations synchronized before tagging.

## Git Workflow
- Do not commit or push unless explicitly requested by the user.
- Keep commits focused and scoped to this repository.

## Performance, Resource Lifecycle & Memory Hygiene
- **Timer & Scheduler Lifecycle:**
  - Always cleanly cancel and stop recurring `time.Timer` or `time.Ticker` instances when stopped or re-registered to avoid leaking goroutines in the WASM runtime.
- **HTTP Client & Response Body Lifecycle:**
  - Reuse `http.Client` with transport connection pooling rather than instantiating new clients per event.
  - Always ensure HTTP response bodies are drained and closed via `defer resp.Body.Close()`.
- **Cache Bounding:**
  - Maintain bounded in-memory caches (with TTL or max item counts) for scrobble tracking and deduplication to keep memory consumption low within the WASM host environment.
