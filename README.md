# tisminSRETool

Linux performance monitoring and observability foundation built with Go.

## Project Goal

Build a maintainable Linux monitoring system that:

- Continuously collects host metrics (CPU, memory, disk, network)
- Supports threshold-based alerting
- Uses context-driven lifecycle control for graceful shutdown
- Evolves into a production-grade observability platform (API, storage, dashboards, diagnostics)

## Current Status

The repository currently provides a working **resident debug runtime** and core collection pipeline.

- Active runtime entry: `cmd/tisminSRETool/debug.go`
- Production entry (`cmd/tisminSRETool/main.go`) is still a placeholder
- Collector is Linux-only and reads from `/proc`
- Alert checker and email sender are wired into the runtime loop
- Diagnostics and HTTP API are not implemented yet

## Architecture (Current)

### Layered design

| Layer | Responsibility | Files |
|---|---|---|
| Entry | Process bootstrap, signal handling, wiring | `cmd/tisminSRETool/debug.go` |
| Engine | Scheduling, context lifecycle, snapshot state, alert orchestration | `internal/engine/runner.go` |
| Collector | Concurrent metric collection orchestration | `internal/collector/linux_collector.go` |
| Linux data source | `/proc` parsing and syscall-based disk stats | `internal/collector/linux_proc.go` |
| Alert | Rule evaluation and notification sending | `internal/alert/interface.go`, `internal/alert/rules.go`, `internal/alert/sender.go` |
| Domain model | Shared data contracts | `internal/model/*.go` |
| Utilities | Context-aware line readers and helpers | `pkg/utils/*.go` |

### Dependency direction

`cmd -> engine -> collector -> utils/model`

`engine -> alert -> model`

This keeps orchestration isolated from low-level metric parsing and makes replacement/refactor easier over time.

## Runtime Flow (Context-driven)

1. Entry creates `rootCtx` using `signal.NotifyContext(SIGINT, SIGTERM)`.
2. Entry constructs `LinuxCollector` and `Runner`.
3. Entry wires alert checker + sender into runner.
4. Runner executes immediate collection once, then continues on ticker (default 5s).
5. Collector launches 4 concurrent jobs: CPU, memory, disk, network.
6. Linux collector functions read `/proc/*` via context-aware readers.
7. Runner stores snapshot (`last metrics`, `last errors`, `last time`).
8. Runner evaluates alert rules and sends email if SMTP config is valid.
9. Entry prints periodic debug summaries from `Runner.Snapshot()`.
10. On signal, context cancellation propagates and all loops stop gracefully.

## Repository Structure

```text
tisminSRETool/
├── cmd/tisminSRETool/
│   ├── main.go                    # reserved production entry (placeholder)
│   └── debug.go                   # current resident runtime entry
├── configs/
│   └── config.yaml                # configuration template (currently minimal)
├── internal/
│   ├── alert/
│   ├── collector/
│   ├── diagnostic/
│   ├── engine/
│   └── model/
├── pkg/utils/
├── docs/
├── go.mod
└── README.md
```

## Run and Validate

### 1. Compile check

```bash
go test ./...
```

### 2. Run resident debug runtime

```bash
go run cmd/tisminSRETool/debug.go
```

The loop runs continuously until SIGINT/SIGTERM.

## Alerting Configuration

Alert rules are initialized in `debug.go` (CPU/memory/disk/network/inodes thresholds).

Email sending is enabled only when these env vars are set:

- `TISMIN_ALERT_SMTP_HOST`
- `TISMIN_ALERT_SMTP_PORT`
- `TISMIN_ALERT_SMTP_USERNAME`
- `TISMIN_ALERT_SMTP_PASSWORD`
- `TISMIN_ALERT_EMAIL_FROM`
- `TISMIN_ALERT_EMAIL_TO` (comma-separated)

Without a complete email config, alert evaluation still runs, but delivery is skipped by design.

## Engineering Rules for Sustainable Growth

### Module boundaries

- Keep `cmd` as wiring only. No business logic in entry files.
- Keep scheduling/state in `engine` only.
- Keep Linux metric parsing in `collector` only.
- Keep alert decision and transport in `alert` only.
- Keep shared contracts in `model` only.

### Context discipline

- Every long-running or IO path must accept `context.Context`.
- Cancellation must stop loops and unblock waits quickly.
- New file-reading logic should use context-aware helpers.

### Error model

- Collection errors are partial by subsystem (`CPU`, `Mem`, `Disk`, `Net`).
- Runner persists both latest metrics and latest errors to avoid data loss.

### Incremental change strategy

- Add new features behind clear interfaces first.
- Keep backward-compatible model changes when possible.
- Prefer small PRs: one module and one behavior change at a time.

### Interface and extension policy

- Keep `collector.Collector`, `alert.AlertChecker`, and `alert.AlertSender` stable.
- Add new capability via new implementations, not by coupling logic into `runner`.
- Introduce adapters when evolving signatures to avoid cross-module breakage.

### Quality gates and release baseline

- Minimum merge gate: `go test ./...` must pass.
- Recommended local gate: `go test -race ./...` before merging runtime changes.
- Every new feature should include at least one testable acceptance path.
- Changes to model fields should update both README and Chinese docs in the same PR.

### Contribution workflow (recommended)

1. Define a single scoped goal (one behavior change).
2. Update interfaces/contracts first, then implementation.
3. Add or update tests.
4. Update documentation (`README.md`, `README_ZH.md`, docs if affected).
5. Merge only when runtime path and rollback path are both clear.

## Roadmap (Recommended Order)

### Phase 1: Production entry and config loading

- Implement `cmd/tisminSRETool/main.go` using `engine.Runner`
- Add config loader (`internal/config`) and remove hardcoded intervals/thresholds

### Phase 2: Observability API

- Add `/healthz` and `/api/v1/metrics/current`
- Expose current runner snapshot through HTTP

### Phase 3: Persistent storage and history

- Add repository/storage interface
- Implement time-series backend (InfluxDB or Prometheus-compatible path)

### Phase 4: Diagnostics and operations

- Implement diagnostic module contracts
- Add runbook and systemd deployment docs

### Phase 5: Testing and quality gates

- Add unit tests for collector parsing and alert rules
- Add integration tests for runner + alert pipeline
- Enforce CI checks (`go test`, lint, race where applicable)

## Known Gaps

- `cmd/tisminSRETool/main.go` is not implemented yet
- `internal/diagnostic/*` is currently placeholder code
- No automated tests yet (`*_test.go` not present)
- Config file is not fully connected to runtime wiring

## Related Docs

- Chinese README: `README_ZH.md`
- Next implementation guide: `docs/NEXT_STEP_IMPLEMENTATION_GUIDE_ZH.md`
- Technical roadmap: `docs/TECH_ROADMAP.md`

## License

MIT
