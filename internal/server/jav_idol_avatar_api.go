package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"javboss/internal/common"
	"javboss/internal/common/logging"
	dbpkg "javboss/internal/db"
	"javboss/internal/manager"
)

const idolAvatarDownloadTimeout = 30 * time.Second

// getJavIdolAvatar serves the selected Gfriends avatar for an idol, downloading it on demand.
func getJavIdolAvatar(c *gin.Context) {
	serveJavIdolAvatar(c, false)
}

// refreshJavIdolAvatar discards cached files and downloads the selection again.
func refreshJavIdolAvatar(c *gin.Context) {
	serveJavIdolAvatar(c, true)
}

func serveJavIdolAvatar(c *gin.Context, refresh bool) {
	id, ok := parseJavIdolID(c)
	if !ok {
		return
	}
	avatarManager, ok := requireIdolAvatarManager(c)
	if !ok {
		return
	}

	c.Header("Cache-Control", "no-cache, must-revalidate")

	ctx, cancel := context.WithTimeout(c.Request.Context(), idolAvatarDownloadTimeout)
	defer cancel()

	names, err := dbpkg.GetJavIdolAvatarNames(ctx, id)
	if err != nil {
		respondIdolAvatarLookupError(c, id, err)
		return
	}
	selectedURL, err := dbpkg.GetJavIdolAvatarURL(ctx, id)
	if err != nil {
		respondIdolAvatarLookupError(c, id, err)
		return
	}

	if refresh {
		avatarManager.Remove(id)
	}

	var avatarPath string
	if selectedURL != "" {
		avatarPath, err = avatarManager.Ensure(ctx, id, selectedURL)
	}
	if selectedURL == "" || err != nil {
		// No explicit selection (or it can no longer be downloaded), fall back to auto.
		if err != nil && selectedURL != "" {
			logging.Info("idol avatar: selected avatar unavailable id=%d: %v", id, err)
		}
		avatarPath, _, err = avatarManager.EnsureAuto(ctx, id, names)
	}
	if err != nil {
		logging.Info("idol avatar unavailable id=%d: %v", id, err)
		respondLocalizedError(c, http.StatusNotFound, "未找到该女优的头像", "No avatar was found for this idol")
		return
	}
	c.File(avatarPath)
}

// listJavIdolAvatarOptions returns every candidate avatar for the idol picker.
func listJavIdolAvatarOptions(c *gin.Context) {
	id, ok := parseJavIdolID(c)
	if !ok {
		return
	}
	avatarManager, ok := requireIdolAvatarManager(c)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), idolAvatarDownloadTimeout)
	defer cancel()

	names, err := dbpkg.GetJavIdolAvatarNames(ctx, id)
	if err != nil {
		respondIdolAvatarLookupError(c, id, err)
		return
	}
	selectedURL, err := dbpkg.GetJavIdolAvatarURL(ctx, id)
	if err != nil {
		respondIdolAvatarLookupError(c, id, err)
		return
	}

	candidates := avatarManager.Candidates(ctx, names...)
	items := make([]gin.H, 0, len(candidates))
	for _, image := range candidates {
		key := manager.CandidateKey(image.URL)
		_, exists := avatarManager.LocalPath(id, image.URL)
		items = append(items, gin.H{
			"key":      key,
			"source":   image.Source,
			"selected": selectedURL != "" && image.URL == selectedURL,
			"exists":   exists,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"items":         items,
		"auto_selected": selectedURL == "",
	})
}

// getJavIdolAvatarCandidate serves one specific candidate image for the picker.
func getJavIdolAvatarCandidate(c *gin.Context) {
	id, ok := parseJavIdolID(c)
	if !ok {
		return
	}
	avatarManager, ok := requireIdolAvatarManager(c)
	if !ok {
		return
	}
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		respondLocalizedError(c, http.StatusBadRequest, "头像标识无效", "Invalid avatar key")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), idolAvatarDownloadTimeout)
	defer cancel()

	names, err := dbpkg.GetJavIdolAvatarNames(ctx, id)
	if err != nil {
		respondIdolAvatarLookupError(c, id, err)
		return
	}
	image, found := avatarManager.FindCandidate(ctx, key, names...)
	if !found {
		respondLocalizedError(c, http.StatusNotFound, "头像不存在", "Avatar was not found")
		return
	}
	avatarPath, err := avatarManager.Ensure(ctx, id, image.URL)
	if err != nil {
		logging.Info("idol avatar candidate unavailable id=%d key=%s: %v", id, key, err)
		respondLocalizedError(c, http.StatusNotFound, "头像下载失败", "Failed to download the avatar")
		return
	}
	c.Header("Cache-Control", "public, max-age=3600")
	c.File(avatarPath)
}

// updateJavIdolAvatar persists the idol's chosen avatar (empty key resets to auto).
func updateJavIdolAvatar(c *gin.Context) {
	id, ok := parseJavIdolID(c)
	if !ok {
		return
	}
	avatarManager, ok := requireIdolAvatarManager(c)
	if !ok {
		return
	}

	var req struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "选择头像请求无效", "Invalid avatar selection request")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), idolAvatarDownloadTimeout)
	defer cancel()

	key := strings.TrimSpace(req.Key)
	if key == "" {
		if err := dbpkg.UpdateJavIdolAvatarURL(ctx, id, ""); err != nil {
			respondJavIdolAvatarSaveError(c, id, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"key": ""})
		return
	}

	names, err := dbpkg.GetJavIdolAvatarNames(ctx, id)
	if err != nil {
		respondIdolAvatarLookupError(c, id, err)
		return
	}
	image, found := avatarManager.FindCandidate(ctx, key, names...)
	if !found {
		respondLocalizedError(c, http.StatusBadRequest, "所选头像无效", "The selected avatar is invalid")
		return
	}
	if err := dbpkg.UpdateJavIdolAvatarURL(ctx, id, image.URL); err != nil {
		respondJavIdolAvatarSaveError(c, id, err)
		return
	}
	// Warm the cache so the avatar endpoint serves immediately next time.
	if _, err := avatarManager.Ensure(ctx, id, image.URL); err != nil {
		logging.Info("idol avatar: warm selection failed id=%d key=%s: %v", id, key, err)
	}
	c.JSON(http.StatusOK, gin.H{"key": key})
}

func parseJavIdolID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		respondLocalizedError(c, http.StatusBadRequest, "女优 ID 无效", "Invalid idol ID")
		return 0, false
	}
	return id, true
}

func requireIdolAvatarManager(c *gin.Context) (*manager.IdolAvatarManager, bool) {
	if common.AppConfig == nil {
		respondLocalizedError(c, http.StatusInternalServerError, "应用配置尚未加载", "Application configuration is not loaded")
		return nil, false
	}
	if common.IdolAvatarManager == nil {
		respondLocalizedError(c, http.StatusNotFound, "女优头像不可用", "Idol avatars are unavailable")
		return nil, false
	}
	return common.IdolAvatarManager, true
}

func respondIdolAvatarLookupError(c *gin.Context, id int64, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		respondLocalizedError(c, http.StatusNotFound, "女优不存在", "Idol was not found")
		return
	}
	logging.Error("load idol avatar names id=%d: %v", id, err)
	respondLocalizedError(c, http.StatusInternalServerError, "加载女优头像失败", "Failed to load the idol avatar")
}

func respondJavIdolAvatarSaveError(c *gin.Context, id int64, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		respondLocalizedError(c, http.StatusNotFound, "女优不存在", "Idol was not found")
		return
	}
	logging.Error("update idol avatar id=%d: %v", id, err)
	respondLocalizedError(c, http.StatusInternalServerError, "保存女优头像失败", "Failed to save the idol avatar")
}
