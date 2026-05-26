package collector

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"tisminSRETool/internal/model"
)

type LinuxCollector struct {
	cpuActive  int32
	memActive  int32
	diskActive int32
	netActive  int32
}

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

	if atomic.CompareAndSwapInt32(&c.cpuActive, 0, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer atomic.StoreInt32(&c.cpuActive, 0)
			
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
	} else {
		errMu.Lock()
		errs.CPU = append(errs.CPU, fmt.Errorf("previous cpu collection is still hanging, skipped"))
		errMu.Unlock()
	}

	if atomic.CompareAndSwapInt32(&c.memActive, 0, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer atomic.StoreInt32(&c.memActive, 0)
			
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
	} else {
		errMu.Lock()
		errs.Mem = append(errs.Mem, fmt.Errorf("previous memory collection is still hanging, skipped"))
		errMu.Unlock()
	}

	if atomic.CompareAndSwapInt32(&c.diskActive, 0, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer atomic.StoreInt32(&c.diskActive, 0)
			
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
	} else {
		errMu.Lock()
		errs.Disk = append(errs.Disk, fmt.Errorf("previous disk collection is still hanging, skipped"))
		errMu.Unlock()
	}

	if atomic.CompareAndSwapInt32(&c.netActive, 0, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer atomic.StoreInt32(&c.netActive, 0)
			
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
	} else {
		errMu.Lock()
		errs.Net = append(errs.Net, fmt.Errorf("previous network collection is still hanging, skipped"))
		errMu.Unlock()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		errMu.Lock()
		// Determine which ones didn't finish purely conceptually, or just report a general timeout
		errs.CPU = append(errs.CPU, fmt.Errorf("collection aborted or timed out: %w", ctx.Err()))
		errMu.Unlock()
	case <-done:
	}

	metrics.UpdateTimestamp = time.Now().Format(time.RFC3339)
	if !errs.HasError() {
		return metrics, nil
	}
	return metrics, errs
}
