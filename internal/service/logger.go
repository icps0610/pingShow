package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"pingShow/internal/models"
	"time"
)

var lastHour = time.Now().Hour()

func StartSnapshotTicker() {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		now := time.Now()
		currentHour := now.Hour()

		models.DataMutex.Lock()
		if currentHour != lastHour {
			for _, t := range models.Targets {
				t.ResetStats()
			}
			lastHour = currentHour
		}

		entry := models.LogEntry{
			Timestamp: now.Format("15:04:05"),
			Metrics:   make(map[string]int64),
		}
		for _, t := range models.Targets {
			entry.Metrics[t.ID] = t.Last
		}
		models.DataMutex.Unlock()

		select {
		case models.LogChan <- entry:
		default:
		}
	}
}

func StartLogWriter() {
	_ = os.MkdirAll("logs", 0755)
	for entry := range models.LogChan {
		now := time.Now()
		fileName := fmt.Sprintf("log_%s_%s.json", now.Format("20060102"), now.Format("15"))
		filePath := filepath.Join("logs", fileName)

		jsonData, err := json.Marshal(entry)
		if err != nil {
			continue
		}

		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			continue
		}
		_, _ = f.Write(append(jsonData, '\n'))
		_ = f.Close()
	}
}
