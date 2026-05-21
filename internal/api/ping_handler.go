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

func HandleRangeHistory(c *gin.Context) {
	endDateStr := c.Query("end_date") // YYYYMMDD
	mode := c.Query("mode")           // "hour", "day_hour", "day" or "month"
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

	if mode == "day_hour" {
		for h := 0; h < 24; h++ {
			targetTime := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), h, 0, 0, 0, models.SystemLocation)
			avgMetrics := calculateHourStat(targetTime, stat)
			if avgMetrics != nil {
				results = append(results, models.LogEntry{
					Timestamp: targetTime.Format("15點"),
					Metrics:   avgMetrics,
				})
			}
		}
	} else if mode == "day" {
		// 依照設定檔的 DayRange，每天一個點
		for i := models.SystemDayRange - 1; i >= 0; i-- {
			targetDate := endDate.AddDate(0, 0, -i)
			avgMetrics := calculateDayStat(targetDate, stat)
			if avgMetrics != nil {
				results = append(results, models.LogEntry{
					Timestamp: targetDate.Format("01/02"),
					Metrics:   avgMetrics,
				})
			}
		}
	} else if mode == "month" {
		// 依照設定檔的 MonthRange，每月一個點
		for i := models.SystemMonthRange - 1; i >= 0; i-- {
			// 抓取該月的第一天到最後一天
			targetMonth := endDate.AddDate(0, -i, 0)
			monthStart := time.Date(targetMonth.Year(), targetMonth.Month(), 1, 0, 0, 0, 0, targetMonth.Location())
			monthEnd := monthStart.AddDate(0, 1, -1)

			// 如果是當月，算到選擇的 endDate 為止
			if i == 0 {
				monthEnd = endDate
			}

			avgMetrics := calculateRangeStat(monthStart, monthEnd, stat)
			if avgMetrics != nil {
				results = append(results, models.LogEntry{
					Timestamp: targetMonth.Format("2006/01"),
					Metrics:   avgMetrics,
				})
			}
		}
	}

	c.JSON(http.StatusOK, results)
}

func calculateHourStat(t time.Time, stat string) map[string]int64 {
	dateStr := t.Format("20060102")
	hourStr := t.Format("15")
	fileName := fmt.Sprintf("log_%s_%s.json", dateStr, hourStr)
	filePath := filepath.Join(models.AppDir, "logs", fileName)

	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	keys := currentTargetIDs()
	values := initMetricValues(keys)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry models.LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			appendEntryMetrics(values, keys, entry)
		}
	}

	return calculateMetricStat(values, keys, stat)
}

func calculateDayStat(date time.Time, stat string) map[string]int64 {
	return calculateRangeStat(date, date, stat)
}

func calculateRangeStat(start, end time.Time, stat string) map[string]int64 {
	keys := currentTargetIDs()
	values := initMetricValues(keys)

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
				}
			}
			file.Close()
		}
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
