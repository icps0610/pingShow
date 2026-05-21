package service

import (
	"pingShow/internal/models"
	"runtime"
	"sort"
	"time"

	ping "github.com/prometheus-community/pro-bing"
)

func StartPingWorker(t *models.TargetInfo) {
	for {
		if !models.IsTargetActive(t.IP) || t.Disabled {
			return // Target was deleted or disabled, stop worker
		}
		pinger, err := ping.NewPinger(t.IP)
		if err != nil {
			updateTimeout(t)
			time.Sleep(time.Duration(models.SystemInterval) * time.Second)
			continue
		}

		pinger.Count = 1
		pinger.Timeout = time.Second
		if runtime.GOOS == "windows" {
			pinger.SetPrivileged(true)
		} else {
			pinger.SetPrivileged(false)
		}

		err = pinger.Run()
		stats := pinger.Statistics()

		models.DataMutex.Lock()
		t.Sent++

		if err != nil || stats.PacketsRecv == 0 {
			t.TO++
			t.Last = -1
			t.AppendHistory(-1)
		} else {
			t.Recv++
			rtt := stats.MinRtt.Milliseconds()

			if t.Last >= 0 {
				j := t.Last - rtt
				if j < 0 {
					j = -j
				}
				t.Jtr = (t.Jtr*9 + j) / 10
			}

			t.Last = rtt
			if rtt < t.Min {
				t.Min = rtt
			}
			if rtt > t.Max {
				t.Max = rtt
			}
			t.TotalRTT += rtt
			t.Avg = t.TotalRTT / int64(t.Recv)
			t.RawRTTs = append(t.RawRTTs, rtt)
			if len(t.RawRTTs) > 3600 {
				t.RawRTTs = t.RawRTTs[1:]
			}
			t.P95 = calculateP95(t.RawRTTs)
			t.AppendHistory(rtt)
		}
		models.DataMutex.Unlock()
		time.Sleep(time.Duration(models.SystemInterval) * time.Second)
	}
}

func updateTimeout(t *models.TargetInfo) {
	models.DataMutex.Lock()
	t.Sent++
	t.TO++
	t.Last = -1
	t.AppendHistory(-1)
	models.DataMutex.Unlock()
}

func calculateP95(rtts []int64) int64 {
	if len(rtts) == 0 {
		return 0
	}
	n := len(rtts)
	sorted := make([]int64, n)
	copy(sorted, rtts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	idx := int(float64(n) * 0.95)
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}
