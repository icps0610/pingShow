package router

import (
	"html/template"
	"pingShow/configs"
	"pingShow/internal/api"

	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 載入嵌入的 HTML 模板 (與 exe 綁定)
	templ := template.Must(template.New("").ParseFS(configs.TemplatesFS, "templates/*.html"))
	r.SetHTMLTemplate(templ)

	// 網頁首頁
	r.GET("/", api.HandleIndex)

	// API 路由群組
	apiGroup := r.Group("/api")
	{
		apiGroup.GET("/metrics", api.HandleMetrics)
		apiGroup.GET("/history", api.HandleHistory)
		apiGroup.GET("/history_range", api.HandleRangeHistory)
		apiGroup.GET("/settings", api.HandleGetSettings)
		apiGroup.POST("/settings", api.HandleSaveSettings)
	}

	return r
}
