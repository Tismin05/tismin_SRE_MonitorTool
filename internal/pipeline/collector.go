package pipeline

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"tisminSRETool/internal/pipeline/proc"
)

// Collector is the data producer that collects metrics from /proc.
type Collector struct {
	dataChan      chan MetricData
	interval      time.Duration
	wg            sync.WaitGroup
	cancel        context.CancelFunc
	prevCPUStat   *proc.CPUStat
	prevSnmpStats *proc.SnmpStats
	mu            sync.Mutex
}

// NewCollector creates a new Collector with the specified channel buffer and interval.
func NewCollector(bufferSize int, interval time.Duration) (*Collector, chan MetricData) {
	ch := make(chan MetricData, bufferSize)
	return &Collector{
		dataChan: ch,
		interval: interval,
	}, ch
}

// Start begins collecting metrics at the specified interval.
// It runs in a goroutine and will stop when ctx is cancelled.
func (c *Collector) Start(ctx context.Context) {
	ctx, c.cancel = context.WithCancel(ctx)

	c.wg.Add(1)
	go c.run(ctx)

	<-ctx.Done()
	c.wg.Wait()
}

// Stop signals the collector to stop.
func (c *Collector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *Collector) run(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Collect immediately on start
	c.collect(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

func (c *Collector) collect(ctx context.Context) {
	host, _ := os.Hostname()
	if host == "" {
		host = "localhost"
	}

	data := MetricData{
		Host: host,
		TS:   time.Now(),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Collect CPU
	wg.Add(1)
	go func() {
		defer wg.Done()
		cpu, err := c.collectCPU(ctx)
		if err != nil {
			log.Printf("[WARN] CPU collection failed: %v", err)
			return
		}
		mu.Lock()
		data.CPU = cpu
		mu.Unlock()
	}()

	// Collect Memory
	wg.Add(1)
	go func() {
		defer wg.Done()
		mem, err := c.collectMem(ctx)
		if err != nil {
			log.Printf("[WARN] Memory collection failed: %v", err)
			return
		}
		mu.Lock()
		data.Mem = mem
		mu.Unlock()
	}()

	// Collect Network
	wg.Add(1)
	go func() {
		defer wg.Done()
		net, err := c.collectNet(ctx)
		if err != nil {
			log.Printf("[WARN] Network collection failed: %v", err)
			return
		}
		mu.Lock()
		data.Net = net
		mu.Unlock()
	}()

	// Collect TCP (new feature)
	wg.Add(1)
	go func() {
		defer wg.Done()
		tcp, err := c.collectTCP(ctx)
		if err != nil {
			log.Printf("[WARN] TCP collection failed: %v", err)
			return
		}
		mu.Lock()
		data.TCP = tcp
		mu.Unlock()
	}()

	// Collect SNMP (new feature)
	wg.Add(1)
	go func() {
		defer wg.Done()
		snmp, err := c.collectSnmp(ctx)
		if err != nil {
			log.Printf("[WARN] SNMP collection failed: %v", err)
			return
		}
		mu.Lock()
		data.Snmp = snmp
		mu.Unlock()
	}()

	wg.Wait()

	select {
	case c.dataChan <- data:
	case <-ctx.Done():
		return
	}
}

func (c *Collector) collectCPU(ctx context.Context) (CPUStat, error) {
	stat, err := proc.ParseCPUStat(ctx)
	if err != nil {
		return CPUStat{}, err
	}

	load, err := proc.ParseLoadAvg(ctx)
	if err != nil {
		return CPUStat{}, err
	}

	usage := 0.0

	c.mu.Lock()
	// DEFENSE: Guard against DeltaTotalTime == 0 (divide by zero)
	// If previous stat exists and Total hasn't changed, usage is 0
	if c.prevCPUStat != nil {
		deltaTotal := stat.Total - c.prevCPUStat.Total
		deltaIdle := stat.Idle - c.prevCPUStat.Idle

		// DEFENSE: deltaTotal == 0 means CPU hasn't ticks, no usage change possible
		if deltaTotal > 0 {
			usage = float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100
			// Clamp to [0, 100]
			if usage < 0 {
				usage = 0
			}
			if usage > 100 {
				usage = 100
			}
		}
	} else {
		// First collection: calculate from absolute values
		if stat.Total > 0 {
			usage = float64(stat.Total-stat.Idle) / float64(stat.Total) * 100
		}
	}

	// Store current stat for next iteration
	c.prevCPUStat = stat
	c.mu.Unlock()

	return CPUStat{
		Cores:        stat.Cores,
		UsagePercent: usage,
		Load1:        load.Load1,
		Load5:        load.Load5,
		Load15:       load.Load15,
	}, nil
}

func (c *Collector) collectMem(ctx context.Context) (MemoryStat, error) {
	mem, err := proc.ParseMemInfo(ctx)
	if err != nil {
		return MemoryStat{}, err
	}

	used := mem.Total - mem.Available
	usedPercent := 0.0
	// DEFENSE: guard against mem.Total == 0
	if mem.Total > 0 {
		usedPercent = float64(used) / float64(mem.Total) * 100
	}

	return MemoryStat{
		Total:       mem.Total,
		Free:        mem.Free,
		Available:   mem.Available,
		Used:        used,
		UsedPercent: usedPercent,
	}, nil
}

func (c *Collector) collectNet(ctx context.Context) ([]NetStat, error) {
	stats, err := proc.ParseNetDev(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]NetStat, len(stats))
	for i, s := range stats {
		result[i] = NetStat{
			Name:      s.Name,
			RxBytes:   s.RxBytes,
			TxBytes:   s.TxBytes,
			RxPackets: s.RxPackets,
			TxPackets: s.TxPackets,
			RxErrors:  s.RxErrors,
			TxErrors:  s.TxErrors,
			RxDropped: s.RxDropped,
			TxDropped: s.TxDropped,
		}
	}

	return result, nil
}

func (c *Collector) collectTCP(ctx context.Context) (TCPStat, error) {
	tcp, err := proc.ParseTCPStat(ctx)
	if err != nil {
		return TCPStat{}, err
	}

	// Convert proc.TCPState to pipeline.TCPState
	states := make(map[TCPState]int)
	for state, count := range tcp.States {
		states[TCPState(state)] = count
	}

	// Also get TCP6 stats and merge
	tcp6, err := proc.ParseTCP6Stat(ctx)
	if err == nil {
		for state, count := range tcp6.States {
			states[TCPState(state)] += count
		}
	}

	return TCPStat{States: states}, nil
}

func (c *Collector) collectSnmp(ctx context.Context) (SnmpStat, error) {
	current, err := proc.ParseSnmpRaw(ctx)
	if err != nil {
		return SnmpStat{}, err
	}

	retransRate := 0.0

	c.mu.Lock()
	if c.prevSnmpStats != nil && current != nil {
		retransRate = proc.CalcRetransRate(current, c.prevSnmpStats)
	}

	// DEFENSE: store previous stats for next rate calculation
	// Make a copy since we're replacing the pointer
	if c.prevSnmpStats == nil {
		c.prevSnmpStats = &proc.SnmpStats{}
	}
	if current != nil {
		*c.prevSnmpStats = *current
	}
	c.mu.Unlock()

	return SnmpStat{
		RetransSegs: current.RetransSegs,
		RetransRate: retransRate,
		OutSegs:     current.OutSegs,
		InSegs:      current.InSegs,
	}, nil
}