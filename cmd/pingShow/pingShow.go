package main

import (
	"fmt"

	"pingShow/internal/models"
	"pingShow/internal/router"
	"pingShow/internal/service"
	"pingShow/pkg/cmdutils"
)

func main() {
	appVersion := `1.0`
	argsCount := 0
	description := `PingShow: Real-time network status monitoring system.
Provides live and historical ping latency charts in a lightweight, background-friendly dashboard.
Customize the web server port and target IPs via target.json.`
	useage := `Usage: pingShow`
	{
		cmdutils.Set(useage, description)
		cmdutils.SetArgs(argsCount, appVersion)
	}

	// "C:\Program Files\Google\Chrome\Application\chrome.exe" --remote-debugging-port=9222 --user-data-dir="z:\Temp\Chrome DevSession"
	{
		// 載入或建立本地監控目標設定檔 (target.json)
		if err := models.LoadTargets(); err != nil {
			fmt.Printf("⚠️ 無法載入 target.json: %v，採用內建監控目標\n", err)
		}

		// 啟動背景服務
		go service.StartLogWriter()

		// 使用 mutex 或複製的 slice 啟動 worker 確保安全
		models.DataMutex.Lock()
		targetsToStart := make([]*models.TargetInfo, len(models.Targets))
		copy(targetsToStart, models.Targets)
		models.DataMutex.Unlock()

		for _, t := range targetsToStart {
			go service.StartPingWorker(t)
		}
		go service.StartSnapshotTicker()

		// 設定與啟動 Gin Router
		r := router.SetupRouter()

		fmt.Println("網路連線狀態即時監測系統已啟動！歷史數據同步寫入 ./logs 夾")
		fmt.Printf("請開啟瀏覽器: http://localhost:%d\n", models.SystemPort)

		if err := r.Run(fmt.Sprintf(":%d", models.SystemPort)); err != nil {
			fmt.Printf("伺服器啟動失敗: %v\n", err)
		}

	}
}
