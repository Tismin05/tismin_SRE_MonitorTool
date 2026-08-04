package collector

import (
	"context"
	"tisminSRETool/internal/model"
)

type Collector interface {
	Collect(ctx context.Context) (*model.Metrics, *model.CollectErrors)
}

// BackgroundCollector is implemented by collectors that need continuously
// sampled data to calculate rates. Runner owns this lifecycle and cancels it
// together with the main collection loop.
type BackgroundCollector interface {
	RunBackground(ctx context.Context)
}
