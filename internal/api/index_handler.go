package api

import (
	"net/http"
	"pingShow/internal/models"

	"github.com/gin-gonic/gin"
)

func HandleIndex(c *gin.Context) {
	models.DataMutex.Lock()
	var activeTargets []*models.TargetInfo
	for _, t := range models.Targets {
		if !t.Disabled {
			activeTargets = append(activeTargets, t)
		}
	}
	models.DataMutex.Unlock()

	c.HTML(http.StatusOK, "index.html", gin.H{
		"Targets":    activeTargets,
		"YMax":       models.SystemYMax,
		"DayRange":   models.SystemDayRange,
		"MonthRange": models.SystemMonthRange,
		"Port":       models.SystemPort,
	})
}
