# tisminSRETool

A Go-based SRE monitoring tool for collecting system metrics (CPU, Memory, Disk, Network), performing system diagnostics, and sending alert notifications.

## Features

- **Linux-focused Support**: Collects metrics from Linux systems
- **Comprehensive Metrics**: CPU, Memory, Disk, Network, Process information
- **System Diagnostics**: Configurable diagnostic checks for system health
- **Alert System**: Configurable thresholds for CPU, Memory, Disk, Network, and Inodes
- **Rate Calculation**: Calculate metrics rate between collection intervals
- **Email Notifications**: SMTP-based alert notification support
- **Context Support**: Graceful shutdown and timeout control via Context

## Project Structure

```
tisminSRETool/
├── cmd/tisminSRETool/          # Program entry points
│   ├── main.go                 # Main program entry (currently empty)
│   └── debug.go                # Debug/Test entry point
├── configs/
│   └── config.yaml             # Configuration file
├── pkg/utils/                  # Public utility packages
│   ├── convert.go              # Unit conversion utilities
│   └── linereader.go           # File line reader with Context support
├── internal/                   # Internal business logic
│   ├── model/                  # Data models
│   │   ├── config.go           # Configuration structs
│   │   ├── metrics.go         # System metrics structs
│   │   ├── diagnostic.go       # Diagnostic result structs
│   │   └── collector_error.go  # Collection error structs
│   ├── collector/              # Metrics collector module
│   │   ├── interface.go       # Collector interface definition
│   │   ├── linux_collector.go # Linux collector entry
│   │   └── linux_proc.go      # Linux collector (/proc filesystem)
│   ├── diagnostic/             # Diagnostic module
│   │   ├── interface.go       # Diagnostic interface
│   │   └── diagnostic_Linux.go # Linux diagnostic implementation
│   ├── alert/                  # Alert module
│   │   ├── interface.go       # Alert interface
│   │   ├── rules.go           # Alert rule checker
│   │   └── sender.go          # Email sender implementation
│   └── engine/                 # Engine module
│       └── calculater.go      # Metrics rate calculation
├── go.mod
├── go.sum
└── README.md
```

## Core Modules

### 1. Collector Module (`internal/collector/`)

Implements system metrics collection via the `Collector` interface:

```go
type Collector interface {
    Collect(ctx context.Context) (*model.Metrics, *model.CollectErrors)
}
```

**Implementations:**
- `LinuxCollector` (`linux_collector.go`): Linux collector entry implementing `Collector`
- Linux proc collector (`linux_proc.go`): Reads Linux metrics directly from `/proc`

**Collected Metrics:**
| Category | Metrics |
|----------|---------|
| CPU | Cores, Usage%, Per-CPU usage, Load (1/5/15 min) |
| Memory | Total, Free, Available, Used, Swap info |
| Disk | Mount points, Capacity, Inodes, IO read/write |
| Network | Interface name, Rx/Tx bytes, packets, errors, drops |
| Processes | PID, Name, CPU%, Memory% |

### 2. Alert Module (`internal/alert/`)

**RuleChecker** (`rules.go`): Checks metrics against configurable thresholds

**Alert Types:**
- CPU usage threshold
- Memory usage threshold
- Disk usage threshold
- Inodes usage threshold
- Network bandwidth/packet loss/latency thresholds
- TCP connection state thresholds (TIME_WAIT, CLOSE_WAIT)

**EmailSender** (`sender.go`): Sends alerts via SMTP

### 3. Engine Module (`internal/engine/`)

**CalculateRate** (`calculater.go`): Calculates rate of change between consecutive collections

- CPU usage rate
- Disk read/write speed (bytes/sec)
- Network Rx/Tx speed (bytes/sec)

### 4. Diagnostic Module (`internal/diagnostic/`)

Interface for system health diagnostics (currently in development).

## Configuration

### config.yaml

```yaml
app:
  name: "tisminSRETool"
  version: "1.0.0"
  refresh_interval: 5s
  loglevel: "info"
  log_path: "./app.log"

diagnostic:
  enabled: true
  show_top_n_list: 10

alert:
  enabled: true
  cpu_threshold: 80.0
  memory_threshold: 80.0
  disk_threshold: 85.0
  inodes_threshold: 80.0
  # ... more thresholds
```

### Configuration Structs (`internal/model/config.go`)

| Struct | Description |
|--------|-------------|
| `Config` | Main configuration container |
| `Appconfig` | Application settings (name, version, interval, logging) |
| `DiagnosticConfig` | Diagnostic module settings |
| `AlertConfig` | Alert thresholds configuration |
| `EmailConfig` | SMTP email configuration |

## Usage

### Quick Start

```bash
# Run debug/test mode
go run cmd/tisminSRETool/debug.go

# Build
go build -o tisminSRETool cmd/tisminSRETool/main.go
```

### Debug Entry Point

The `debug.go` file provides a testing interface:

```go
func main() {
    c := &collector.LinuxCollector{}
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    metrics, collectErrs := c.Collect(ctx)
    // Output metrics as JSON
}
```

## Development

### Adding New Metrics

1. Add new fields to `internal/model/metrics.go`
2. Implement collection logic in collector implementations
3. Use goroutines for parallel collection

### Adding Alert Rules

1. Add threshold fields to `AlertConfig` in `internal/model/config.go`
2. Implement check logic in `internal/alert/rules.go`
3. Add configuration in `configs/config.yaml`

### Platform Support

Create Linux-specific collector helpers under `internal/collector/`.

## Dependencies

- [gopsutil](https://github.com/shirou/gopsutil): Cross-platform system metrics
- Go 1.25.6+

## License

MIT

---

[中文版](./README_ZH.md)
