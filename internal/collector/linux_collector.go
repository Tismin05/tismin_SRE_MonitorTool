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
		c.cpuSampler = &cpuSampler{}
		c.netSampler = &netSampler{}
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
		value, err := collectCPUStat(ctx, c.cpuSampler)
		results <- collectionResult{subsystem: "cpu", cpu: value, err: err}
	}()
	go func() {
		defer wg.Done()
		value, err := CollectMeminfo(ctx)
		if err == nil && value == nil {
			err = fmt.Errorf("memory stat is nil")
		}
		results <- collectionResult{subsystem: "memory", mem: value, err: err}
	}()
	go func() {
		defer wg.Done()
		value, err := CollectDisk(ctx)
		results <- collectionResult{subsystem: "disk", disk: value, err: err}
	}()
	go func() {
		defer wg.Done()
		value, err := collectNetinfo(ctx, c.netSampler)
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
