package engine

import (
	"context"
	"io"
	"log"
	"sync/atomic"
	"testing"
	"time"

	"tisminSRETool/internal/model"
)

type runnerTestCollector struct {
	calls   atomic.Int32
	called  chan struct{}
	metrics *model.Metrics
}

type lifecycleTestCollector struct {
	collectStarted    chan struct{}
	backgroundStarted chan struct{}
	backgroundStopped chan struct{}
}

func (c *lifecycleTestCollector) Collect(ctx context.Context) (*model.Metrics, *model.CollectErrors) {
	close(c.collectStarted)
	<-ctx.Done()
	return nil, &model.CollectErrors{CPU: []error{ctx.Err()}}
}

func (c *lifecycleTestCollector) RunBackground(ctx context.Context) {
	close(c.backgroundStarted)
	<-ctx.Done()
	close(c.backgroundStopped)
}

func (c *runnerTestCollector) Collect(context.Context) (*model.Metrics, *model.CollectErrors) {
	c.calls.Add(1)
	select {
	case c.called <- struct{}{}:
	default:
	}
	return c.metrics, nil
}

func TestRunnerWaitsForCollectionAndBackgroundShutdown(t *testing.T) {
	collector := &lifecycleTestCollector{
		collectStarted:    make(chan struct{}),
		backgroundStarted: make(chan struct{}),
		backgroundStopped: make(chan struct{}),
	}
	runner := NewRunner(collector, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go runner.Run(ctx)

	for name, started := range map[string]<-chan struct{}{
		"collection": collector.collectStarted,
		"background": collector.backgroundStarted,
	} {
		select {
		case <-started:
		case <-time.After(time.Second):
			cancel()
			t.Fatalf("%s did not start", name)
		}
	}

	cancel()
	select {
	case <-runner.Done():
	case <-time.After(time.Second):
		t.Fatal("runner did not complete cancellation")
	}
	select {
	case <-collector.backgroundStopped:
	default:
		t.Fatal("Runner.Done closed before the background sampler stopped")
	}
}

func TestRunnerLifecycleAndSnapshotIsolation(t *testing.T) {
	collector := &runnerTestCollector{
		called: make(chan struct{}, 1),
		metrics: &model.Metrics{
			Host: "host-a",
			CPU:  model.CPUStat{PerCPUUsage: []float64{10, 20}},
			Disk: []model.DiskStat{{Device: "sda"}},
		},
	}
	runner := NewRunner(collector, time.Hour, log.New(io.Discard, "", 0))
	ctx, cancel := context.WithCancel(context.Background())
	go runner.Run(ctx)

	select {
	case <-collector.called:
	case <-time.After(time.Second):
		t.Fatal("runner did not perform its immediate collection")
	}

	first, _, _ := runner.Snapshot()
	if first == nil || first.Host != "host-a" {
		t.Fatalf("unexpected snapshot: %#v", first)
	}
	first.CPU.PerCPUUsage[0] = 99
	first.Disk[0].Device = "mutated"
	collector.metrics.CPU.PerCPUUsage[0] = 88
	collector.metrics.Disk[0].Device = "source-mutated"

	second, _, _ := runner.Snapshot()
	if second.CPU.PerCPUUsage[0] != 10 || second.Disk[0].Device != "sda" {
		t.Fatal("Snapshot exposed Runner's internal mutable state")
	}

	cancel()
	select {
	case <-runner.Done():
	case <-time.After(time.Second):
		t.Fatal("runner did not stop after cancellation")
	}
	if runner.IsRunning() {
		t.Fatal("runner still reports running after Done was closed")
	}

	returned := make(chan struct{})
	go func() {
		runner.Run(context.Background())
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("a second Run call did not return")
	}
}
