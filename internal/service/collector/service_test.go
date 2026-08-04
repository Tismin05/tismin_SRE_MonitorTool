package collectorservice

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"tisminSRETool/internal/model"
)

type serviceTestCollector struct {
	called chan struct{}
}

func (c *serviceTestCollector) Collect(context.Context) (*model.Metrics, *model.CollectErrors) {
	select {
	case c.called <- struct{}{}:
	default:
	}
	return &model.Metrics{Host: "host-a"}, nil
}

func TestServiceStopsAllComponentsWhenContextIsCanceled(t *testing.T) {
	collector := &serviceTestCollector{called: make(chan struct{}, 1)}
	cfg := &model.Config{
		App: model.AppConfig{RefreshInterval: time.Hour},
	}
	service, err := New(
		cfg,
		log.New(io.Discard, "", 0),
		WithCollector(collector),
		WithShutdownTimeout(time.Second),
	)
	if err != nil {
		t.Fatalf("New returned an error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()

	select {
	case <-collector.called:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("service did not start its collector")
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Service.Run returned an error on normal cancellation: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("service did not shut down")
	}
}
