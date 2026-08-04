package collector

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"tisminSRETool/internal/model"
)

func TestLinuxCollectorWaitsForWorkersAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var active atomic.Int32

	waitForRelease := func(ctx context.Context) error {
		active.Add(1)
		defer active.Add(-1)
		started <- struct{}{}
		<-ctx.Done()
		<-release
		return ctx.Err()
	}

	c := &LinuxCollector{
		collectCPU: func(ctx context.Context, _ *cpuSampler) (model.CPUStat, error) {
			return model.CPUStat{}, waitForRelease(ctx)
		},
		collectMem: func(ctx context.Context) (*model.MemoryStat, error) {
			return nil, waitForRelease(ctx)
		},
		collectDisk: func(ctx context.Context, _ *diskSampler) ([]model.DiskStat, error) {
			return nil, waitForRelease(ctx)
		},
		collectNet: func(ctx context.Context, _ *netSampler) ([]model.NetStat, error) {
			_ = waitForRelease(ctx)
			return []model.NetStat{{Name: "eth0", RxBytes: 42}}, nil
		},
	}

	type result struct {
		metrics *model.Metrics
		errs    *model.CollectErrors
	}
	returned := make(chan result, 1)
	go func() {
		metrics, errs := c.Collect(ctx)
		returned <- result{metrics: metrics, errs: errs}
	}()

	for range 4 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not all start")
		}
	}
	cancel()

	select {
	case <-returned:
		t.Fatal("Collect returned before canceled workers exited")
	case <-time.After(30 * time.Millisecond):
	}

	close(release)
	select {
	case got := <-returned:
		if got.errs == nil || len(got.errs.Context) != 1 || !errors.Is(got.errs.Context[0], context.Canceled) ||
			len(got.errs.CPU) != 1 || len(got.errs.Mem) != 1 || len(got.errs.Disk) != 1 ||
			!errors.Is(got.errs.CPU[0], context.Canceled) ||
			!errors.Is(got.errs.Mem[0], context.Canceled) ||
			!errors.Is(got.errs.Disk[0], context.Canceled) || len(got.errs.Net) != 0 {
			t.Fatalf("cancellation errors not preserved: %#v", got.errs)
		}
		if got.metrics == nil || len(got.metrics.Net) != 1 || got.metrics.Net[0].RxBytes != 42 {
			t.Fatalf("successful partial result was not preserved: %#v", got.metrics)
		}
	case <-time.After(time.Second):
		t.Fatal("Collect did not return after workers exited")
	}
	if got := active.Load(); got != 0 {
		t.Fatalf("Collect returned with %d active workers", got)
	}
}

func TestLinuxCollectorSamplersAreIsolated(t *testing.T) {
	first := &LinuxCollector{}
	second := &LinuxCollector{}
	first.initSamplers()
	second.initSamplers()

	if first.cpuSampler == second.cpuSampler || first.netSampler == second.netSampler || first.diskSampler == second.diskSampler {
		t.Fatal("collector instances share sampler pointers")
	}

	now := time.Now()
	first.cpuSampler.buffer.snapshots[0] = cpuCacheSnapshot{
		timestamp: now,
		perCPU:    []cpuCacheSnapshot{{total: 100, idle: 20}},
	}
	first.netSampler.buffer.snapshots[0] = []netSnapshot{{timestamp: now, iface: "eth0", rxBytes: 100}}

	cpu0, cpu1 := second.cpuSampler.snapshots()
	if !cpu0.timestamp.IsZero() || !cpu1.timestamp.IsZero() {
		t.Fatal("second collector observed first collector CPU samples")
	}
	net0, net1, _, _ := second.netSampler.snapshots()
	if len(net0) != 0 || len(net1) != 0 {
		t.Fatal("second collector observed first collector network samples")
	}
}

func TestLinuxCollectorAttributesCancellationOnlyToFailingWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &LinuxCollector{
		collectCPU: func(context.Context, *cpuSampler) (model.CPUStat, error) {
			return model.CPUStat{Cores: 2}, nil
		},
		collectMem: func(context.Context) (*model.MemoryStat, error) {
			return &model.MemoryStat{Total: 10}, nil
		},
		collectDisk: func(ctx context.Context, _ *diskSampler) ([]model.DiskStat, error) {
			return nil, ctx.Err()
		},
		collectNet: func(context.Context, *netSampler) ([]model.NetStat, error) {
			return []model.NetStat{{Name: "eth0"}}, nil
		},
	}

	metrics, collectErrs := c.Collect(ctx)
	if collectErrs == nil || len(collectErrs.Context) != 1 || !errors.Is(collectErrs.Context[0], context.Canceled) ||
		len(collectErrs.Disk) != 1 || !errors.Is(collectErrs.Disk[0], context.Canceled) {
		t.Fatalf("disk cancellation was not preserved: %#v", collectErrs)
	}
	if len(collectErrs.CPU) != 0 || len(collectErrs.Mem) != 0 || len(collectErrs.Net) != 0 {
		t.Fatalf("cancellation was attributed to successful workers: %#v", collectErrs)
	}
	if metrics.CPU.Cores != 2 || metrics.Mem.Total != 10 || len(metrics.Net) != 1 {
		t.Fatalf("successful worker data was not preserved: %#v", metrics)
	}
}

func TestCPUSamplerSnapshotsDeepCopyPerCPU(t *testing.T) {
	sampler := &cpuSampler{}
	sampler.buffer.snapshots[0] = cpuCacheSnapshot{
		timestamp: time.Now(),
		perCPU:    []cpuCacheSnapshot{{total: 100, idle: 25}},
	}

	first, _ := sampler.snapshots()
	first.perCPU[0].total = 999

	again, _ := sampler.snapshots()
	if got := again.perCPU[0].total; got != 100 {
		t.Fatalf("snapshot mutation reached sampler buffer: got total %v", got)
	}
}
