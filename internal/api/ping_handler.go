package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"pingShow/internal/models"

	"github.com/gin-gonic/gin"
)

func HandleMetrics(c *gin.Context) {
	models.DataMutex.Lock()
	defer models.DataMutex.Unlock()
	c.JSON(http.StatusOK, models.Targets)
}

func HandleHistory(c *gin.Context) {
	date := c.Query("date")
	hour := c.Query("hour")
	if date == "" || hour == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少參數"})
		return
	}

	fileName := fmt.Sprintf("log_%s_%s.json", date, hour)
	filePath := filepath.Join("logs", fileName)

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
	mode := c.Query("mode")           // "week" or "month"

	if endDateStr == "" || mode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少參數"})
		return
	}

	endDate, err := time.Parse("20060102", endDateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "日期格式錯誤"})
		return
	}

	var results []models.LogEntry

	if mode == "day" {
		// 往前推 30 天，每天一個點
		for i := 29; i >= 0; i-- {
			targetDate := endDate.AddDate(0, 0, -i)
			avgMetrics := calculateDayAverage(targetDate)
			if avgMetrics != nil {
				results = append(results, models.LogEntry{
					Timestamp: targetDate.Format("01/02"),
					Metrics:   avgMetrics,
				})
			}
		}
	} else if mode == "month" {
		// 往前推 12 個月，每月一個點
		for i := 11; i >= 0; i-- {
			// 抓取該月的第一天到最後一天
			targetMonth := endDate.AddDate(0, -i, 0)
			monthStart := time.Date(targetMonth.Year(), targetMonth.Month(), 1, 0, 0, 0, 0, targetMonth.Location())
			monthEnd := monthStart.AddDate(0, 1, -1)
			
			// 如果是當月，算到選擇的 endDate 為止
			if i == 0 {
				monthEnd = endDate
			}
			
			avgMetrics := calculateRangeAverage(monthStart, monthEnd)
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

func calculateDayAverage(date time.Time) map[string]int64 {
	return calculateRangeAverage(date, date)
}

func calculateRangeAverage(start, end time.Time) map[string]int64 {
	var sums = make(map[string]int64)
	var counts = make(map[string]int64)
	var keys []string

	models.DataMutex.Lock()
	for _, t := range models.Targets {
		keys = append(keys, t.ID)
		sums[t.ID] = 0
		counts[t.ID] = 0
	}
	models.DataMutex.Unlock()

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("20060102")
		for h := 0; h < 24; h++ {
			fileName := fmt.Sprintf("log_%s_%02d.json", dateStr, h)
			filePath := filepath.Join("logs", fileName)

			file, err := os.Open(filePath)
			if err != nil {
				continue
			}

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				var entry models.LogEntry
				if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
					for _, id := range keys {
						val, ok := entry.Metrics[id]
						if !ok {
							continue
						}
						if val == -1 {
							continue // skip timeout for average calculation
						}
						sums[id] += val
						counts[id]++
					}
				}
			}
			file.Close()
		}
	}

	result := make(map[string]int64)
	valid := false
	for _, id := range keys {
		if counts[id] > 0 {
			result[id] = sums[id] / counts[id]
			valid = true
		} else {
			result[id] = 0
		}
	}

	if !valid {
		return nil
	}

	return result
}

