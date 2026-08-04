package collector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"tisminSRETool/internal/model"
)

type LinuxCollector struct {
	samplersOnce sync.Once
	cpuSampler   *cpuSampler
	netSampler   *netSampler

	collectCPU  func(context.Context, *cpuSampler) (model.CPUStat, error)
	collectMem  func(context.Context) (*model.MemoryStat, error)
	collectDisk func(context.Context) ([]model.DiskStat, error)
	collectNet  func(context.Context, *netSampler) ([]model.NetStat, error)
}

var _ Collector = (*LinuxCollector)(nil)
var _ BackgroundCollector = (*LinuxCollector)(nil)

// RunBackground maintains the short-window CPU and network samples used to
// calculate utilization and transfer rates. It blocks until ctx is canceled.
func (c *LinuxCollector) RunBackground(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	c.initSamplers()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		c.cpuSampler.run(ctx, 100*time.Millisecond)
	}()
	go func() {
		defer wg.Done()
		c.netSampler.run(ctx, 100*time.Millisecond)
	}()
	wg.Wait()
}

func (c *LinuxCollector) initSamplers() {
	c.samplersOnce.Do(func() {
		if c.cpuSampler == nil {
			c.cpuSampler = &cpuSampler{}
		}
		if c.netSampler == nil {
			c.netSampler = &netSampler{}
		}
		if c.collectCPU == nil {
			c.collectCPU = collectCPUStat
		}
		if c.collectMem == nil {
			c.collectMem = CollectMeminfo
		}
		if c.collectDisk == nil {
			c.collectDisk = CollectDisk
		}
		if c.collectNet == nil {
			c.collectNet = collectNetinfo
		}
	})
}

type collectionResult struct {
	subsystem string
	cpu       model.CPUStat
	mem       *model.MemoryStat
	disk      []model.DiskStat
	net       []model.NetStat
	err       error
}

func (c *LinuxCollector) Collect(ctx context.Context) (*model.Metrics, *model.CollectErrors) {
	if ctx == nil {
		ctx = context.Background()
	}
	c.initSamplers()

	host := "localhost"
	if h, err := os.Hostname(); err == nil && h != "" {
		host = h
	}

	metrics := &model.Metrics{
		Host:            host,
		UpdateTimestamp: time.Now().Format(time.RFC3339),
	}
	errs := &model.CollectErrors{}
	results := make(chan collectionResult, 4)

	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		value, err := c.collectCPU(ctx, c.cpuSampler)
		results <- collectionResult{subsystem: "cpu", cpu: value, err: err}
	}()
	go func() {
		defer wg.Done()
		value, err := c.collectMem(ctx)
		if err == nil && value == nil {
			err = fmt.Errorf("memory stat is nil")
		}
		results <- collectionResult{subsystem: "memory", mem: value, err: err}
	}()
	go func() {
		defer wg.Done()
		value, err := c.collectDisk(ctx)
		results <- collectionResult{subsystem: "disk", disk: value, err: err}
	}()
	go func() {
		defer wg.Done()
		value, err := c.collectNet(ctx, c.netSampler)
		results <- collectionResult{subsystem: "network", net: value, err: err}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		if result.err != nil {
			appendCollectError(errs, result.subsystem, result.err)
			continue
		}
		switch result.subsystem {
		case "cpu":
			metrics.CPU = result.cpu
		case "memory":
			metrics.Mem = *result.mem
		case "disk":
			metrics.Disk = result.disk
		case "network":
			metrics.Net = result.net
		}
	}
	if err := ctx.Err(); err != nil {
		appendContextError(errs, err)
	}

	metrics.UpdateTimestamp = time.Now().Format(time.RFC3339)
	if !errs.HasError() {
		return metrics, nil
	}
	return metrics, errs
}

// appendContextError makes collection cancellation observable even when a
// worker manages to return partial data without propagating ctx.Err itself.
func appendContextError(errs *model.CollectErrors, err error) {
	if !containsError(errs.CPU, err) {
		errs.CPU = append(errs.CPU, err)
	}
	if !containsError(errs.Mem, err) {
		errs.Mem = append(errs.Mem, err)
	}
	if !containsError(errs.Disk, err) {
		errs.Disk = append(errs.Disk, err)
	}
	if !containsError(errs.Net, err) {
		errs.Net = append(errs.Net, err)
	}
}

func containsError(errs []error, target error) bool {
	for _, err := range errs {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func appendCollectError(errs *model.CollectErrors, subsystem string, err error) {
	switch subsystem {
	case "cpu":
		errs.CPU = append(errs.CPU, err)
	case "memory":
		errs.Mem = append(errs.Mem, err)
	case "disk":
		errs.Disk = append(errs.Disk, err)
	case "network":
		errs.Net = append(errs.Net, err)
	}
}
