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
	// 往前推 30 天
	start := now.AddDate(0, 0, -30)
	
	// 把 start 調整到該日的 00:00:00
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())

	fmt.Println("開始產生測試資料，時間範圍：", start.Format("2006-01-02"), "至", now.Format("2006-01-02"))

	rand.Seed(time.Now().UnixNano())

	// 依照小時迴圈
	for t := start; t.Before(now); t = t.Add(1 * time.Hour) {
		dateStr := t.Format("20060102")
		hourStr := t.Format("15")
		
		fileName := fmt.Sprintf("log_%s_%s.json", dateStr, hourStr)
		filePath := filepath.Join("logs", fileName)

		f, err := os.Create(filePath)
		if err != nil {
			fmt.Println("無法建立檔案:", err)
			return
		}

		// 每個小時有 3600 秒
		for sec := 0; sec < 3600; sec++ {
			// 如果超過現在時間就停止
			currentSec := t.Add(time.Duration(sec) * time.Second)
			if currentSec.After(now) {
				break
			}

			// 隨機生成波動的 RTT，偶爾產生 timeout (-1)
			local := int64(rand.Intn(5) + 1)
			matsu := int64(rand.Intn(20) + 30)
			tw_dns := int64(rand.Intn(10) + 5)
			line := int64(rand.Intn(40) + 40)

			// 模擬偶發性 Timeout 或尖峰 (1% 機率)
			if rand.Intn(100) < 1 {
				matsu = -1
			} else if rand.Intn(100) < 2 {
				matsu += 150 // 尖峰
			}

			if rand.Intn(100) < 1 {
				line = -1
			}

			entry := LogEntry{
				Timestamp: currentSec.Format("15:04:05"),
				Metrics: map[string]int64{
					"local":  local,
					"matsu":  matsu,
					"tw_dns": tw_dns,
					"line":   line,
				},
			}

			jsonData, _ := json.Marshal(entry)
			f.Write(append(jsonData, '\n'))
		}
		f.Close()
	}

	fmt.Println("測試資料產生完畢！")
}
