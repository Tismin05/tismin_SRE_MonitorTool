package collector

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
	"tisminSRETool/internal/model"
)

type LinuxCollector struct{}

var _ Collector = (*LinuxCollector)(nil)

func (c *LinuxCollector) Collect(ctx context.Context) (*model.Metrics, *model.CollectErrors) {
	host := "localhost"
	if h, err := os.Hostname(); err == nil && h != "" {
		host = h
	}

	metrics := &model.Metrics{
		Host:            host,
		UpdateTimestamp: time.Now().Format(time.RFC3339),
	}
	errs := &model.CollectErrors{}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errMu sync.Mutex

	wg.Add(4)

	go func() {
		defer wg.Done()
		cpuStat, err := CollectCPUStat(ctx)
		if err != nil {
			errMu.Lock()
			errs.CPU = append(errs.CPU, err)
			errMu.Unlock()
			return
		}
		mu.Lock()
		metrics.CPU = cpuStat
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		memStat, err := CollectMeminfo(ctx)
		if err != nil {
			errMu.Lock()
			errs.Mem = append(errs.Mem, err)
			errMu.Unlock()
			return
		}
		if memStat == nil {
			errMu.Lock()
			errs.Mem = append(errs.Mem, fmt.Errorf("memory stat is nil"))
			errMu.Unlock()
			return
		}
		mu.Lock()
		metrics.Mem = *memStat
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		diskStat, err := CollectDisk(ctx)
		if err != nil {
			errMu.Lock()
			errs.Disk = append(errs.Disk, err)
			errMu.Unlock()
			return
		}
		mu.Lock()
		metrics.Disk = diskStat
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		netStat, err := CollectNetinfo(ctx)
		if err != nil {
			errMu.Lock()
			errs.Net = append(errs.Net, err)
			errMu.Unlock()
			return
		}
		mu.Lock()
		metrics.Net = netStat
		mu.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		errMu.Lock()
		errs.CPU = append(errs.CPU, ctx.Err())
		errMu.Unlock()
	case <-done:
	}

	metrics.UpdateTimestamp = time.Now().Format(time.RFC3339)
	if !errs.HasError() {
		return metrics, nil
	}
	return metrics, errs
}
