package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

type LogEntry struct {
	Timestamp string           `json:"timestamp"`
	Metrics   map[string]int64 `json:"metrics"`
}

func main() {
	_ = os.MkdirAll("logs", 0755)

	now := time.Now()
	start := now.AddDate(-1, 0, 0) // 往前推一年 (365天)
	
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())

	fmt.Println("開始產生測試資料，時間範圍：", start.Format("2006-01-02"), "至", now.Format("2006-01-02"))

	rand.Seed(time.Now().UnixNano())

	// 依照天迴圈，每天只產生中午 12 點的日誌 (大幅加快速度且足以測試區間平均)
	for t := start; t.Before(now); t = t.AddDate(0, 0, 1) {
		dateStr := t.Format("20060102")
		
		fileName := fmt.Sprintf("log_%s_12.json", dateStr)
		filePath := filepath.Join("logs", fileName)

		f, err := os.Create(filePath)
		if err != nil {
			fmt.Println("無法建立檔案:", err)
			return
		}

		// 每天產生 5 筆紀錄就足夠測試平均值
		for sec := 0; sec < 5; sec++ {
			local := int64(rand.Intn(5) + 1)
			line := int64(rand.Intn(40) + 40)
			taiwan := int64(rand.Intn(10) + 5)
			google := int64(rand.Intn(15) + 10)

			// 模擬偶發性 Timeout (-1)
			if rand.Intn(100) < 5 {
				line = -1
			}

			entry := LogEntry{
				Timestamp: fmt.Sprintf("12:0%d:00", sec),
				Metrics: map[string]int64{
					"192.168.0.1":    local,
					"access.line.me": line,
					"168.95.1.1":     taiwan,
					"8.8.8.8":        google,
				},
			}

			jsonData, _ := json.Marshal(entry)
			f.Write(append(jsonData, '\n'))
		}
		f.Close()
	}

	fmt.Println("測試資料產生完畢！")
}
