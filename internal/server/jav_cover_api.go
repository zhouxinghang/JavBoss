package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"javboss/internal/common"
	"javboss/internal/common/logging"
	"javboss/internal/manager"
)

// getJavCover serves a downloaded JAV cover if present; otherwise enqueues and returns 404.
func getJavCover(c *gin.Context) {
	code := c.Param("code")
	cfg := common.AppConfig
	if cfg == nil {
		respondLocalizedError(c, http.StatusInternalServerError, "应用配置尚未加载", "Application configuration is not loaded")
		return
	}

	c.Header("Cache-Control", "no-cache, must-revalidate")

	if path, ok := manager.FindCoverPath(cfg.JavCoverDir, code); ok {
		c.File(path)
		return
	}

	if common.CoverManager != nil {
		common.CoverManager.Enqueue(code)
	}
	respondLocalizedError(c, http.StatusNotFound, "JAV 封面不存在", "JAV cover was not found")
}

func updateJavCover(c *gin.Context) {
	code := strings.TrimSpace(c.Param("code"))
	cfg := common.AppConfig
	if cfg == nil {
		respondLocalizedError(c, http.StatusInternalServerError, "应用配置尚未加载", "Application configuration is not loaded")
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "更新 JAV 封面请求无效", "Invalid JAV cover update request")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	if err := manager.DownloadCoverFromURL(ctx, cfg.JavCoverDir, code, req.URL); err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "下载 JAV 封面失败，请检查图片地址", "Failed to download the JAV cover; check the image URL")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": strings.ToLower(code)})
}

// updateJavCoverFromScreenshot re-stores a video screenshot as the JAV cover file.
// The cover stays a plain file so no database structure changes are needed.
func updateJavCoverFromScreenshot(c *gin.Context) {
	code := strings.TrimSpace(c.Param("code"))
	cfg := common.AppConfig
	if cfg == nil {
		respondLocalizedError(c, http.StatusInternalServerError, "应用配置尚未加载", "Application configuration is not loaded")
		return
	}

	var req struct {
		VideoID        int64  `json:"video_id"`
		ScreenshotName string `json:"screenshot_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "更新 JAV 封面请求无效", "Invalid JAV cover update request")
		return
	}

	name := filepath.Base(strings.TrimSpace(req.ScreenshotName))
	if req.VideoID <= 0 || !isScreenshotImageName(name) || name != strings.TrimSpace(req.ScreenshotName) {
		respondLocalizedError(c, http.StatusBadRequest, "截图文件名无效", "Invalid screenshot filename")
		return
	}

	dataDir := filepath.Dir(cfg.DatabasePath)
	screenshotPath := filepath.Join(dataDir, "video", strconv.FormatInt(req.VideoID, 10), "screenshot", name)
	if _, err := os.Stat(screenshotPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			respondLocalizedError(c, http.StatusNotFound, "用于封面的截图不存在", "The screenshot selected as cover does not exist")
			return
		}
		logging.Error("stat JAV cover screenshot error: %v", err)
		respondLocalizedError(c, http.StatusInternalServerError, "读取封面截图失败", "Failed to inspect the cover screenshot")
		return
	}

	if err := manager.StoreCoverFromFile(cfg.JavCoverDir, code, screenshotPath); err != nil {
		logging.Error("store JAV cover from screenshot error: %v", err)
		respondLocalizedError(c, http.StatusBadRequest, "保存 JAV 封面失败", "Failed to save the JAV cover")
		return
	}
	c.Header("Cache-Control", "no-cache, must-revalidate")
	c.JSON(http.StatusOK, gin.H{"code": strings.ToLower(code)})
}
