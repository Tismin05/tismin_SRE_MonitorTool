package collector

import (
	"context"
	"log"
	"strconv"
	"strings"
	"tisminSRETool/internal/model"
	"tisminSRETool/pkg/utils"
)

// CollectMeminfo 采集内存信息
func CollectMeminfo(ctx context.Context) (m *model.MemoryStat, err error) {
	lines, err := utils.ReadLinesOffsetNWithContext(ctx, "/proc/meminfo", 0, -1)
	if err != nil {
		log.Printf("error collecting meminfo: %s", err)
		return nil, err
	}

	ret := &model.MemoryStat{}
	var buffer uint64
	var cache uint64
	var available uint64
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fields := strings.SplitN(line, ":", 2)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSpace(fields[0])
		valueFields := strings.Fields(strings.TrimSpace(fields[1]))
		if len(valueFields) == 0 {
			continue
		}
		value := valueFields[0]
		switch key {
		case "MemTotal":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting MemTotal: %s", err)
				return ret, err
			}
			ret.Total = t * 1024
		case "MemFree":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting MemFree: %s", err)
				return ret, err
			}
			ret.Free = t * 1024
		case "MemAvailable":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting MemAvailable: %s", err)
				return ret, err
			}
			available = t * 1024
		case "SwapTotal":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting SwapTotal: %s", err)
				return ret, err
			}
			ret.SwapTotal = t * 1024
		case "SwapFree":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting SwapFree: %s", err)
				return ret, err
			}
			ret.SwapFree = t * 1024
		case "Buffers":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting Buffers: %s", err)
				return ret, err
			}
			buffer = t * 1024
		case "Cached":
			t, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				log.Printf("error collecting Cached: %s", err)
				return ret, err
			}
			cache = t * 1024
		default:
			continue
		}
	}
	ret.Available = available
	if available > 0 {
		ret.Used = ret.Total - available
	} else {
		ret.Used = ret.Total - ret.Free - buffer - cache
	}
	if ret.Total > 0 {
		ret.UsedPercent = float64(ret.Used) / float64(ret.Total) * 100
	}
	if ret.SwapTotal >= ret.SwapFree {
		ret.SwapUsed = ret.SwapTotal - ret.SwapFree
		if ret.SwapTotal > 0 {
			ret.SwapUsedPercent = float64(ret.SwapUsed) / float64(ret.SwapTotal) * 100
		}
	}
	return ret, nil
}
