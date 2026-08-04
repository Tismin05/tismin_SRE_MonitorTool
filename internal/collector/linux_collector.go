package collector

import (
	"context"
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
	diskSampler  *diskSampler

	collectCPU  func(context.Context, *cpuSampler) (model.CPUStat, error)
	collectMem  func(context.Context) (*model.MemoryStat, error)
	collectDisk func(context.Context, *diskSampler) ([]model.DiskStat, error)
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
		if c.diskSampler == nil {
			c.diskSampler = &diskSampler{}
		}
		if c.collectCPU == nil {
			c.collectCPU = collectCPUStat
		}
		if c.collectMem == nil {
			c.collectMem = CollectMeminfo
		}
		if c.collectDisk == nil {
			c.collectDisk = collectDisk
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
		value, err := c.collectDisk(ctx, c.diskSampler)
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
		switch result.subsystem {
		case "cpu":
			metrics.CPU = result.cpu
		case "memory":
			if result.mem != nil {
				metrics.Mem = *result.mem
			}
		case "disk":
			metrics.Disk = result.disk
		case "network":
			metrics.Net = result.net
		}
		if result.err != nil {
			appendCollectError(errs, result.subsystem, result.err)
		}
	}
	if err := ctx.Err(); err != nil {
		errs.Context = append(errs.Context, err)
	}
	metrics.UpdateTimestamp = time.Now().Format(time.RFC3339)
	if !errs.HasError() {
		return metrics, nil
	}
	return metrics, errs
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
