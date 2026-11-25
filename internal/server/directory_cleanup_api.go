package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"javboss/internal/common/logging"
	dbpkg "javboss/internal/db"
	"javboss/internal/models"
	"javboss/internal/service"
)

func loadDirectoryForCleanup(c *gin.Context) (*models.Directory, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "目录 ID 无效", "Invalid directory ID")
		return nil, false
	}
	dir, err := dbpkg.GetDirectory(c.Request.Context(), id)
	if err != nil {
		logging.Error("get directory for cleanup failed id=%d err=%v", id, err)
		respondLocalizedError(c, http.StatusInternalServerError, "读取目录失败", "Failed to load directory")
		return nil, false
	}
	if dir == nil || dir.IsDelete {
		respondLocalizedError(c, http.StatusNotFound, "目录不存在", "Directory does not exist")
		return nil, false
	}
	if dir.Missing {
		respondLocalizedError(c, http.StatusConflict, "目录当前不可用", "The directory is currently unavailable")
		return nil, false
	}
	return dir, true
}

func scanDirectoryCleanup(c *gin.Context) {
	dir, ok := loadDirectoryForCleanup(c)
	if !ok {
		return
	}

	switch service.DirectoryWorkStatus(dir.ID) {
	case service.DirectoryWorkOrganizing,
		service.DirectoryWorkGeneratingSidecar,
		service.DirectoryWorkOrganizingWithSidecar:
		respondLocalizedError(c, http.StatusConflict, "目录正在执行整理任务，请稍后重试", "The directory is currently organizing; please try again later")
		return
	}

	var req struct {
		Categories        *[]string `json:"categories"`
		IncludeDuplicates *bool     `json:"include_duplicates"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		respondLocalizedError(c, http.StatusBadRequest, "清理扫描请求无效", "Invalid cleanup scan request")
		return
	}

	categories := service.AllJunkCategories()
	if req.Categories != nil {
		categories = *req.Categories
	}
	includeDuplicates := true
	if req.IncludeDuplicates != nil {
		includeDuplicates = *req.IncludeDuplicates
	}

	normalizedCategories, err := service.NormalizeCleanupCategories(categories)
	if err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "包含不支持的垃圾文件扫描类别", "Request contains an unsupported junk scan category")
		return
	}
	if len(normalizedCategories) == 0 && !includeDuplicates {
		respondLocalizedError(c, http.StatusBadRequest, "请至少选择一个垃圾文件类别或开启重复文件筛选", "Select at least one junk category or enable duplicate file filtering")
		return
	}

	result, err := service.ScanDirectoryCleanup(c.Request.Context(), *dir, service.DirectoryCleanupScanOptions{
		Categories:        normalizedCategories,
		IncludeDuplicates: includeDuplicates,
	})
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			return
		case errors.Is(err, os.ErrNotExist):
			respondLocalizedError(c, http.StatusConflict, "目录当前不可用", "The directory is currently unavailable")
		case errors.Is(err, os.ErrPermission):
			respondLocalizedError(c, http.StatusForbidden, "没有权限读取该目录", "Permission denied when reading this directory")
		default:
			logging.Error("scan directory cleanup failed id=%d path=%s err=%v", dir.ID, dir.Path, err)
			respondLocalizedError(c, http.StatusInternalServerError, "扫描目录失败", "Failed to scan the directory")
		}
		return
	}
	c.JSON(http.StatusOK, result)
}

func deleteDirectoryCleanupFiles(c *gin.Context) {
	dir, ok := loadDirectoryForCleanup(c)
	if !ok {
		return
	}

	var req struct {
		Paths           []string `json:"paths"`
		RemoveEmptyDirs bool     `json:"remove_empty_dirs"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "清理删除请求无效", "Invalid cleanup delete request")
		return
	}
	hasNonEmpty := false
	for _, p := range req.Paths {
		if strings.TrimSpace(p) != "" {
			hasNonEmpty = true
			break
		}
	}
	if !hasNonEmpty {
		respondLocalizedError(c, http.StatusBadRequest, "请至少选择一个要删除的文件", "Select at least one file to delete")
		return
	}

	result, err := service.DeleteDirectoryCleanupFiles(
		c.Request.Context(),
		*dir,
		req.Paths,
		req.RemoveEmptyDirs,
	)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDirectoryWorkInProgress),
			errors.Is(err, service.ErrDirectoryScanInProgress),
			errors.Is(err, context.DeadlineExceeded):
			respondLocalizedError(c, http.StatusConflict, "目录正在执行其他任务，请稍后重试", "The directory is busy; please try again later")
		case errors.Is(err, os.ErrNotExist):
			respondLocalizedError(c, http.StatusConflict, "目录当前不可用", "The directory is currently unavailable")
		default:
			logging.Error("delete directory cleanup files failed id=%d path=%s err=%v", dir.ID, dir.Path, err)
			respondLocalizedError(c, http.StatusInternalServerError, "清理文件失败", "Failed to clean up files")
		}
		return
	}
	c.JSON(http.StatusOK, result)
}

func previewDirectoryCleanupFile(c *gin.Context) {
	dir, ok := loadDirectoryForCleanup(c)
	if !ok {
		return
	}

	relPath := strings.TrimSpace(c.Query("path"))
	if relPath == "" {
		respondLocalizedError(c, http.StatusBadRequest, "缺少文件路径参数", "File path query parameter is required")
		return
	}

	target, contentType, err := service.ResolveDirectoryCleanupPreviewPath(*dir, relPath)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCleanupFileUnsupported):
			respondLocalizedError(c, http.StatusBadRequest, "该文件类型不支持预览", "This file type does not support preview")
		case errors.Is(err, service.ErrCleanupFileUnavailable), errors.Is(err, os.ErrNotExist):
			respondLocalizedError(c, http.StatusNotFound, "文件不存在或无法预览", "File does not exist or cannot be previewed")
		default:
			logging.Error("preview directory cleanup file failed id=%d path=%s err=%v", dir.ID, relPath, err)
			respondLocalizedError(c, http.StatusInternalServerError, "读取预览图片失败", "Failed to load preview image")
		}
		return
	}

	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, max-age=60")
	c.Header("Content-Type", contentType)
	c.File(target)
}
