# tisminSRETool 下一步实施文档（详细过程 + 代码）

## 1. 目标与范围

当前仓库已经切换为 **Linux-only**，并且 `collector` 可以编译通过。  
下一步要把项目从「debug 单次采集」推进到「可长期运行的监控服务」。

本实施文档按 5 个阶段推进：

1. 建立主运行链路（`main + runner + ticker`）
2. 建立配置加载层（统一 `config.yaml` 与 `model.Config`）
3. 打通告警链路（规则检查 + 发送）
4. 暴露可观测性接口（健康检查 + 当前指标）
5. Linux 部署（systemd）

---

## 2. 阶段 1：主运行链路（先跑起来）

### 2.1 目标

- 程序可常驻运行
- 使用 `signal.NotifyContext` 优雅退出
- 按固定周期采集指标
- 内存中保留最近一次采集结果

### 2.2 新增文件

- `internal/engine/runner.go`

```go
package engine

import (
	"context"
	"log"
	"sync"
	"time"
	"tisminSRETool/internal/collector"
	"tisminSRETool/internal/model"
)

type Runner struct {
	collector collector.Collector
	interval  time.Duration
	logger    *log.Logger

	mu       sync.RWMutex
	last     *model.Metrics
	lastErrs *model.CollectErrors
	lastAt   time.Time
}

func NewRunner(c collector.Collector, interval time.Duration, logger *log.Logger) *Runner {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Runner{
		collector: c,
		interval:  interval,
		logger:    logger,
	}
}

func (r *Runner) Run(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.collectOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			if r.logger != nil {
				r.logger.Printf("runner stopped: %v", ctx.Err())
			}
			return
		case <-ticker.C:
			r.collectOnce(ctx)
		}
	}
}

func (r *Runner) collectOnce(parent context.Context) {
	if r.collector == nil {
		if r.logger != nil {
			r.logger.Printf("collect skipped: collector is nil")
		}
		return
	}

	collectCtx, cancel := context.WithTimeout(parent, r.interval)
	defer cancel()

	metrics, errs := r.collector.Collect(collectCtx)

	r.mu.Lock()
	r.last = metrics
	r.lastErrs = errs
	r.lastAt = time.Now()
	r.mu.Unlock()

	if r.logger == nil {
		return
	}

	if errs != nil && errs.HasError() {
		r.logger.Printf("collect finished with errors: %+v", errs)
		return
	}

	if metrics == nil {
		r.logger.Printf("collect finished with empty metrics")
		return
	}

	r.logger.Printf("collect finished: host=%s ts=%s", metrics.Host, metrics.UpdateTimestamp)
}

func (r *Runner) Snapshot() (metrics *model.Metrics, errs *model.CollectErrors, at time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.last, r.lastErrs, r.lastAt
}
```

### 2.3 修改 `cmd/tisminSRETool/main.go`

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"tisminSRETool/internal/collector"
	"tisminSRETool/internal/engine"
)

func main() {
	logger := log.New(os.Stdout, "[tisminSRETool] ", log.LstdFlags|log.Lshortfile)

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c := &collector.LinuxCollector{}
	r := engine.NewRunner(c, 5*time.Second, logger)
	r.Run(rootCtx)
}
```

### 2.4 验收命令

```bash
go test ./...
go run cmd/tisminSRETool/main.go
```

---

## 3. 阶段 2：配置层（可维护）

### 3.1 目标

- 采集间隔、告警阈值由配置文件控制
- `config.yaml` 与 `model.Config` 字段对齐

### 3.2 依赖

```bash
go get github.com/spf13/viper
```

### 3.3 新增 `internal/config/loader.go`

```go
package config

import (
	"fmt"
	"time"
	"tisminSRETool/internal/model"

	"github.com/spf13/viper"
)

func Load(path string) (*model.Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	v.SetDefault("app.name", "tisminSRETool")
	v.SetDefault("app.version", "0.1.0")
	v.SetDefault("app.refresh_interval", "5s")
	v.SetDefault("app.loglevel", "info")
	v.SetDefault("app.log_path", "./app.log")

	v.SetDefault("alert.enabled", true)
	v.SetDefault("alert.cpu_threshold", 80.0)
	v.SetDefault("alert.memory_threshold", 80.0)
	v.SetDefault("alert.disk_threshold", 85.0)
	v.SetDefault("alert.inodes_threshold", 80.0)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config failed: %w", err)
	}

	var cfg model.Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config failed: %w", err)
	}

	if cfg.App.RefreshInterval <= 0 {
		cfg.App.RefreshInterval = 5 * time.Second
	}
	return &cfg, nil
}
```

### 3.4 `main.go` 切到配置驱动

```go
cfg, err := config.Load("configs/config.yaml")
if err != nil {
	logger.Fatalf("load config failed: %v", err)
}
r := engine.NewRunner(c, cfg.App.RefreshInterval, logger)
```

---

## 4. 阶段 3：告警链路（采集 -> 规则 -> 发送）

### 4.1 目标

- 每次采集后执行规则检查
- 触发告警后调用 sender 发送

### 4.2 先统一接口签名（关键）

`internal/alert/interface.go` 目标签名：

```go
type AlertChecker interface {
	Check(ctx context.Context, m *model.Metrics) ([]Alert, error)
}

type AlertSender interface {
	Send(ctx context.Context, alerts []Alert, cfg model.EmailConfig) error
}
```

### 4.3 在 `Runner` 中接入 checker/sender

给 `Runner` 增加字段：

```go
checker alert.AlertChecker
sender  alert.AlertSender
email   model.EmailConfig
```

然后在 `collectOnce` 中加：

```go
if r.checker != nil && metrics != nil {
	alerts, err := r.checker.Check(parent, metrics)
	if err != nil {
		r.logger.Printf("alert check failed: %v", err)
	} else if len(alerts) > 0 && r.sender != nil {
		if err := r.sender.Send(parent, alerts, r.email); err != nil {
			r.logger.Printf("alert send failed: %v", err)
		}
	}
}
```

---

## 5. 阶段 4：可观测性 API（先做最小集）

### 5.1 目标

- 提供健康检查
- 提供当前指标快照

### 5.2 新增 `internal/server/http.go`

```go
package server

import (
	"encoding/json"
	"net/http"
	"time"
	"tisminSRETool/internal/engine"
)

type HTTPServer struct {
	runner *engine.Runner
}

func NewHTTPServer(r *engine.Runner) *HTTPServer {
	return &HTTPServer{runner: r}
}

func (s *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _, at := s.runner.Snapshot()
		status := map[string]any{
			"status":      "ok",
			"last_collect": at.Format(time.RFC3339),
		}
		_ = json.NewEncoder(w).Encode(status)
	})

	mux.HandleFunc("/api/v1/metrics/current", func(w http.ResponseWriter, r *http.Request) {
		metrics, errs, at := s.runner.Snapshot()
		resp := map[string]any{
			"metrics": metrics,
			"errors":  errs,
			"at":      at.Format(time.RFC3339),
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	return mux
}
```

### 5.3 在 `main.go` 并发启动 HTTP + Runner

- 一个 goroutine 运行 `runner.Run(ctx)`
- 一个 goroutine 跑 `http.Server.ListenAndServe()`
- `ctx.Done()` 后 `server.Shutdown()`

---

## 6. 阶段 5：Linux 部署（systemd）

### 6.1 构建

```bash
go build -o tisminSRETool cmd/tisminSRETool/main.go
```

### 6.2 systemd 文件示例 `/etc/systemd/system/tisminsretool.service`

```ini
[Unit]
Description=tisminSRETool
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/tisminSRETool
ExecStart=/opt/tisminSRETool/tisminSRETool
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

### 6.3 启动

```bash
sudo systemctl daemon-reload
sudo systemctl enable tisminsretool
sudo systemctl start tisminsretool
sudo systemctl status tisminsretool
```

---

## 7. 每阶段验收清单（建议）

### 阶段 1 验收

- `go test ./...` 通过
- `main.go` 可常驻运行并可 Ctrl+C 退出

### 阶段 2 验收

- 改 `refresh_interval` 后采集频率可变化

### 阶段 3 验收

- 人工构造超阈值数据可触发告警发送

### 阶段 4 验收

- `/healthz` 返回 `status=ok`
- `/api/v1/metrics/current` 返回最新指标

### 阶段 5 验收

- `systemctl restart` 后服务自动恢复
- 服务器重启后服务自动拉起

---

## 8. 建议的立即执行顺序（本周可完成）

1. 先完成阶段 1（今天）
2. 同步完成阶段 2（今天）
3. 然后修正 alert 接口并接入阶段 3（明天）
4. 最后补阶段 4 的最小 HTTP 接口（明天）

完成这 4 步后，项目就从“代码原型”进入“可部署、可运行、可扩展”的基础状态。
