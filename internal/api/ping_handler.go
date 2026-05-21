package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"pingShow/internal/models"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

func HandleMetrics(c *gin.Context) {
	models.DataMutex.Lock()
	var activeTargets []*models.TargetInfo
	for _, t := range models.Targets {
		if !t.Disabled {
			activeTargets = append(activeTargets, t)
		}
	}
	models.DataMutex.Unlock()
	c.JSON(http.StatusOK, activeTargets)
}

func HandleHistory(c *gin.Context) {
	date := c.Query("date")
	hour := c.Query("hour")
	if date == "" || hour == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少參數"})
		return
	}

	fileName := fmt.Sprintf("log_%s_%s.json", date, hour)
	filePath := filepath.Join(models.AppDir, "logs", fileName)

	file, err := os.Open(filePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "找不到該時段的紀錄檔案"})
		return
	}
	defer file.Close()

	var entries []models.LogEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry models.LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			entries = append(entries, entry)
		}
	}

	c.JSON(http.StatusOK, entries)
}

type GlobalStat struct {
	Min int64 `json:"min"`
	Max int64 `json:"max"`
	Avg int64 `json:"avg"`
	Jtr int64 `json:"jtr"`
	P95 int64 `json:"p95"`
	To  int64 `json:"to"`
}

func mergeValues(dest, src map[string]*metricSample, keys []string) {
	if src == nil {
		return
	}
	for _, id := range keys {
		dest[id].Raw = append(dest[id].Raw, src[id].Raw...)
		dest[id].Valid = append(dest[id].Valid, src[id].Valid...)
	}
}

func HandleRangeHistory(c *gin.Context) {
	endDateStr := c.Query("end_date") // YYYYMMDD
	mode := c.Query("mode")           // "day_hour", "day" or "month"
	stat := c.DefaultQuery("stat", "avg")

	if endDateStr == "" || mode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少參數"})
		return
	}

	endDate, err := time.ParseInLocation("20060102", endDateStr, models.SystemLocation)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "日期格式錯誤"})
		return
	}

	var results []models.LogEntry
	keys := currentTargetIDs()
	globalValues := initMetricValues(keys)

	if mode == "day_hour" {
		for h := 0; h < 24; h++ {
			targetTime := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), h, 0, 0, 0, models.SystemLocation)
			
			// 預設空的 metrics，皆填入 -1 代表無資料
			avgMetrics := make(map[string]int64)
			for _, id := range keys {
				avgMetrics[id] = -1
			}

			hourVals := getHourValues(targetTime, keys)
			if hourVals != nil {
				mergeValues(globalValues, hourVals, keys)
				if m := calculateMetricStat(hourVals, keys, stat); m != nil {
					avgMetrics = m
				}
			}

			// 不管有沒有資料，都推入結果，確保 X 軸完整 00~23
			results = append(results, models.LogEntry{
				Timestamp: targetTime.Format("15點"),
				Metrics:   avgMetrics,
			})
		}
	} else if mode == "day" {
		for i := models.SystemDayRange - 1; i >= 0; i-- {
			targetDate := endDate.AddDate(0, 0, -i)
			dayVals := getRangeValues(targetDate, targetDate, keys)
			if dayVals != nil {
				mergeValues(globalValues, dayVals, keys)
				avgMetrics := calculateMetricStat(dayVals, keys, stat)
				if avgMetrics != nil {
					results = append(results, models.LogEntry{
						Timestamp: targetDate.Format("01/02"),
						Metrics:   avgMetrics,
					})
				}
			}
		}
	} else if mode == "month" {
		for i := models.SystemMonthRange - 1; i >= 0; i-- {
			targetMonth := endDate.AddDate(0, -i, 0)
			monthStart := time.Date(targetMonth.Year(), targetMonth.Month(), 1, 0, 0, 0, 0, targetMonth.Location())
			monthEnd := monthStart.AddDate(0, 1, -1)
			if i == 0 {
				monthEnd = endDate
			}

			monthVals := getRangeValues(monthStart, monthEnd, keys)
			if monthVals != nil {
				mergeValues(globalValues, monthVals, keys)
				avgMetrics := calculateMetricStat(monthVals, keys, stat)
				if avgMetrics != nil {
					results = append(results, models.LogEntry{
						Timestamp: targetMonth.Format("2006/01"),
						Metrics:   avgMetrics,
					})
				}
			}
		}
	}

	stats := make(map[string]*GlobalStat)
	for _, id := range keys {
		sample := globalValues[id]
		if len(sample.Raw) == 0 {
			stats[id] = &GlobalStat{}
			continue
		}

		minVal, maxVal, avgVal := int64(0), int64(0), int64(0)
		if len(sample.Valid) > 0 {
			minVal = sample.Valid[0]
			maxVal = sample.Valid[0]
			var sum int64
			for _, v := range sample.Valid {
				if v < minVal {
					minVal = v
				}
				if v > maxVal {
					maxVal = v
				}
				sum += v
			}
			avgVal = sum / int64(len(sample.Valid))
		}

		toCount := int64(0)
		for _, v := range sample.Raw {
			if v == -1 {
				toCount++
			}
		}

		jtrCount := int64(0)
		sumJtr := int64(0)
		var lastValid int64 = -1
		for _, v := range sample.Raw {
			if v != -1 {
				if lastValid != -1 {
					diff := v - lastValid
					if diff < 0 {
						diff = -diff
					}
					sumJtr += diff
					jtrCount++
				}
				lastValid = v
			} else {
				lastValid = -1
			}
		}
		jtrVal := int64(0)
		if jtrCount > 0 {
			jtrVal = sumJtr / jtrCount
		}

		p95Val := int64(0)
		if len(sample.Valid) > 0 {
			p95Val = percentile(sample.Valid, 0.95)
		}

		stats[id] = &GlobalStat{
			Min: minVal,
			Max: maxVal,
			Avg: avgVal,
			Jtr: jtrVal,
			P95: p95Val,
			To:  toCount,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": results,
		"stats":   stats,
	})
}

func getHourValues(t time.Time, keys []string) map[string]*metricSample {
	dateStr := t.Format("20060102")
	hourStr := t.Format("15")
	fileName := fmt.Sprintf("log_%s_%s.json", dateStr, hourStr)
	filePath := filepath.Join(models.AppDir, "logs", fileName)

	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	values := initMetricValues(keys)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry models.LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			appendEntryMetrics(values, keys, entry)
		}
	}
	return values
}

func getRangeValues(start, end time.Time, keys []string) map[string]*metricSample {
	values := initMetricValues(keys)
	found := false
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("20060102")
		for h := 0; h < 24; h++ {
			fileName := fmt.Sprintf("log_%s_%02d.json", dateStr, h)
			filePath := filepath.Join(models.AppDir, "logs", fileName)

			file, err := os.Open(filePath)
			if err != nil {
				continue
			}

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				var entry models.LogEntry
				if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
					appendEntryMetrics(values, keys, entry)
					found = true
				}
			}
			file.Close()
		}
	}
	if !found {
		return nil
	}
	return values
}

func calculateHourStat(t time.Time, stat string) map[string]int64 {
	keys := currentTargetIDs()
	values := getHourValues(t, keys)
	if values == nil {
		return nil
	}
	return calculateMetricStat(values, keys, stat)
}

func calculateDayStat(date time.Time, stat string) map[string]int64 {
	return calculateRangeStat(date, date, stat)
}

func calculateRangeStat(start, end time.Time, stat string) map[string]int64 {
	keys := currentTargetIDs()
	values := getRangeValues(start, end, keys)
	if values == nil {
		return nil
	}
	return calculateMetricStat(values, keys, stat)
}

func currentTargetIDs() []string {
	var keys []string

	models.DataMutex.Lock()
	for _, t := range models.Targets {
		if !t.Disabled {
			keys = append(keys, t.ID)
		}
	}
	models.DataMutex.Unlock()

	return keys
}

type metricSample struct {
	Raw   []int64
	Valid []int64
}

func initMetricValues(keys []string) map[string]*metricSample {
	values := make(map[string]*metricSample, len(keys))
	for _, id := range keys {
		values[id] = &metricSample{}
	}
	return values
}

func appendEntryMetrics(values map[string]*metricSample, keys []string, entry models.LogEntry) {
	for _, id := range keys {
		val, ok := entry.Metrics[id]
		if !ok {
			continue
		}
		values[id].Raw = append(values[id].Raw, val)
		if val != -1 {
			values[id].Valid = append(values[id].Valid, val)
		}
	}
}

func calculateMetricStat(values map[string]*metricSample, keys []string, stat string) map[string]int64 {
	result := make(map[string]int64)
	valid := false
	for _, id := range keys {
		sample := values[id]
		if !hasMetricData(sample, stat) {
			result[id] = 0
			continue
		}

		result[id] = aggregateMetric(sample, stat)
		valid = true
	}

	if !valid {
		return nil
	}

	return result
}

func hasMetricData(sample *metricSample, stat string) bool {
	if stat == "timeout" {
		return len(sample.Raw) > 0
	}
	if stat == "jitter" {
		return len(sample.Raw) > 0
	}
	return len(sample.Valid) > 0
}

func aggregateMetric(sample *metricSample, stat string) int64 {
	switch stat {
	case "p95":
		return percentile(sample.Valid, 0.95)
	case "timeout":
		var timeouts int64
		for _, val := range sample.Raw {
			if val == -1 {
				timeouts++
			}
		}
		return (timeouts*100 + int64(len(sample.Raw))/2) / int64(len(sample.Raw))
	case "jitter":
		return averageJitter(sample.Raw)
	}

	var sum int64
	for _, val := range sample.Valid {
		sum += val
	}
	return sum / int64(len(sample.Valid))
}

func averageJitter(values []int64) int64 {
	var last int64
	hasLast := false
	var sum int64
	var count int64
	for _, val := range values {
		if val == -1 {
			continue
		}
		if hasLast {
			diff := val - last
			if diff < 0 {
				diff = -diff
			}
			sum += diff
			count++
		}
		last = val
		hasLast = true
	}
	if count == 0 {
		return 0
	}
	return (sum + count/2) / count
}

func percentile(values []int64, p float64) int64 {
	sorted := make([]int64, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	idx := int(float64(len(sorted)) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
