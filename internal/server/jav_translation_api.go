package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"javboss/internal/common/logging"
	"javboss/internal/db"
	"javboss/internal/deepseek"
	"javboss/internal/service/enrichment"
)

// createJavTitleTranslation returns a draft translation; it is persisted through
// PUT /jav/items/:id only when the user saves the editor.
func createJavTitleTranslation(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	var req struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "标题翻译请求无效", "Invalid title translation request")
		return
	}
	config, err := db.ListConfig(c.Request.Context())
	if err != nil {
		respondLocalizedError(c, http.StatusInternalServerError, "读取翻译设置失败", "Failed to load translation settings")
		return
	}
	zhTitle, err := enrichment.TranslateTitle(c.Request.Context(), deepseek.SettingsFromConfig(config), req.Title)
	if err != nil {
		switch {
		case errors.Is(err, deepseek.ErrNotConfigured):
			respondLocalizedError(c, http.StatusBadRequest, "请先在全局设置的标题翻译中配置 DeepSeek API Key", "Configure a DeepSeek API key in global title translation settings first")
		case errors.Is(err, deepseek.ErrInvalidTitle):
			respondLocalizedError(c, http.StatusBadRequest, "原始标题不能为空且不能超过 5000 字", "Original title must contain between 1 and 5000 characters")
		default:
			logging.Error("translate jav title: %v", err)
			respondLocalizedError(c, http.StatusBadGateway, "DeepSeek 翻译失败，请检查 API Key、余额、模型和网络后重试", "DeepSeek translation failed; check your API key, balance, model and network, then retry")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"zh_title": zhTitle})
}
