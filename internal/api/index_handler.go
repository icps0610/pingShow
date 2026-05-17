package api

import (
	"net/http"
	"pingShow/internal/models"

	"github.com/gin-gonic/gin"
)

func HandleIndex(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", gin.H{
		"Targets":    models.Targets,
		"YMax":       models.SystemYMax,
		"DayRange":   models.SystemDayRange,
		"MonthRange": models.SystemMonthRange,
		"Port":       models.SystemPort,
	})
}
