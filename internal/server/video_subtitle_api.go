package server

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"javboss/internal/common"
	"javboss/internal/common/logging"
	dbpkg "javboss/internal/db"
	"javboss/internal/subtitle"
)

func resolveVideoSubtitle(c *gin.Context) (int64, string, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "视频 ID 无效", "Invalid video ID")
		return 0, "", false
	}
	video, err := dbpkg.GetVideo(c.Request.Context(), id)
	if err != nil {
		respondSubtitleError(c, err)
		return 0, "", false
	}
	if video == nil {
		respondLocalizedError(c, http.StatusNotFound, "视频不存在", "Video does not exist")
		return 0, "", false
	}
	if common.AppConfig == nil {
		respondLocalizedError(c, http.StatusInternalServerError, "应用配置尚未加载", "Application configuration is not loaded")
		return 0, "", false
	}
	return id, filepath.Dir(common.AppConfig.DatabasePath), true
}

func getVideoSubtitle(c *gin.Context) {
	id, dataDir, ok := resolveVideoSubtitle(c)
	if !ok {
		return
	}
	file, err := subtitle.Load(dataDir, id)
	if err != nil {
		respondSubtitleError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"subtitle": file})
}

func getVideoSubtitleFile(c *gin.Context) {
	id, dataDir, ok := resolveVideoSubtitle(c)
	if !ok {
		return
	}
	file, err := subtitle.Load(dataDir, id)
	if err != nil {
		respondSubtitleError(c, err)
		return
	}
	if file == nil {
		respondLocalizedError(c, http.StatusNotFound, "尚未上传字幕", "No subtitle has been uploaded")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Type", "application/octet-stream")
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(file.Path)
}

func uploadVideoSubtitle(c *gin.Context) {
	id, dataDir, ok := resolveVideoSubtitle(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, subtitle.MaxSize+(64<<10))
	if err := c.Request.ParseMultipartForm(subtitle.MaxSize); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			respondSubtitleError(c, subtitle.ErrTooLarge)
		} else {
			respondLocalizedError(c, http.StatusBadRequest, "请选择字幕文件", "Choose a subtitle file")
		}
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "请选择字幕文件", "Choose a subtitle file")
		return
	}
	defer file.Close()
	saved, err := subtitle.Save(dataDir, id, header.Filename, file)
	if err != nil {
		respondSubtitleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"subtitle": saved})
}

func deleteVideoSubtitle(c *gin.Context) {
	id, dataDir, ok := resolveVideoSubtitle(c)
	if !ok {
		return
	}
	if err := subtitle.Remove(dataDir, id); err != nil {
		respondSubtitleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func respondSubtitleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, subtitle.ErrTooLarge):
		respondLocalizedError(c, http.StatusRequestEntityTooLarge, "字幕文件不能超过 10 MB", "Subtitle files must be 10 MB or smaller")
	case errors.Is(err, subtitle.ErrInvalid):
		respondLocalizedError(c, http.StatusBadRequest, "字幕内容或格式无效，请上传 SRT、ASS、SSA 或 VTT 字幕", "Invalid subtitle content or format; upload an SRT, ASS, SSA or VTT subtitle")
	default:
		logging.Error("video subtitle error: %v", err)
		respondLocalizedError(c, http.StatusInternalServerError, "读取或保存字幕失败，请重试", "Could not read or save the subtitle; try again")
	}
}
