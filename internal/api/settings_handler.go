package api

import (
	"net/http"
	"pingShow/internal/models"
	"pingShow/internal/service"

	"github.com/gin-gonic/gin"
)

type TargetInput struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

type SettingsInput struct {
	Port       int           `json:"port"`
	Interval   int           `json:"interval"`
	YMax       int           `json:"ymax"`
	DayRange   int           `json:"day_range"`
	MonthRange int           `json:"month_range"`
	Targets    []TargetInput `json:"targets"`
}

func HandleGetSettings(c *gin.Context) {
	models.DataMutex.Lock()
	defer models.DataMutex.Unlock()

	targets := []TargetInput{}
	for _, t := range models.Targets {
		targets = append(targets, TargetInput{Name: t.Name, IP: t.IP})
	}

	c.JSON(http.StatusOK, SettingsInput{
		Port:       models.SystemPort,
		Interval:   models.SystemInterval,
		YMax:       models.SystemYMax,
		DayRange:   models.SystemDayRange,
		MonthRange: models.SystemMonthRange,
		Targets:    targets,
	})
}

func HandleSaveSettings(c *gin.Context) {
	var input SettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 驗證輸入參數
	if input.Port <= 0 {
		input.Port = 80
	}
	if input.Interval <= 0 {
		input.Interval = 1
	}
	if input.YMax <= 0 {
		input.YMax = 50
	}
	if input.DayRange <= 0 {
		input.DayRange = 7
	}
	if input.MonthRange <= 0 {
		input.MonthRange = 3
	}
	if len(input.Targets) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "監控目標不能為空"})
		return
	}

	models.DataMutex.Lock()
	
	// 更新系統組態變數
	models.SystemPort = input.Port
	models.SystemInterval = input.Interval
	models.SystemYMax = input.YMax
	models.SystemDayRange = input.DayRange
	models.SystemMonthRange = input.MonthRange

	// 動態調整 Targets 列表
	oldTargetsMap := make(map[string]*models.TargetInfo)
	for _, t := range models.Targets {
		oldTargetsMap[t.IP] = t
	}

	// 收集需要啟動的新目標，先不啟動 worker
	newTargets := []*models.TargetInfo{}
	newWorkers := []*models.TargetInfo{}
	seenIP := make(map[string]bool)

	for _, inputT := range input.Targets {
		if inputT.IP == "" {
			continue
		}
		if seenIP[inputT.IP] {
			continue // 去重
		}
		seenIP[inputT.IP] = true

		if oldT, ok := oldTargetsMap[inputT.IP]; ok {
			// 保留的原有目標，僅更新名稱
			oldT.Name = inputT.Name
			newTargets = append(newTargets, oldT)
		} else {
			// 新增目標
			newT := &models.TargetInfo{
				ID:   inputT.IP,
				Name: inputT.Name,
				IP:   inputT.IP,
				Min:  9999,
			}
			newTargets = append(newTargets, newT)
			newWorkers = append(newWorkers, newT) // 記下要啟動 worker
		}
	}

	models.Targets = newTargets
	models.DataMutex.Unlock()

	// 釋放鎖之後，再啟動新的 ping worker，避免 goroutine 內再次取鎖造成競爭
	for _, newT := range newWorkers {
		go service.StartPingWorker(newT)
	}

	// 寫回 target.json
	if err := models.SaveTargets(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "儲存設定失敗: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "設定已成功儲存"})
}
