package collector

import (
	"context"
	"tisminSRETool/internal/model"
)

type Collector interface {
	Collect(ctx context.Context) (*model.Metrics, error)
}
