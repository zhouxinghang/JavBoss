package server

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
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

// idolAvatarUploadLimitBytes mirrors the manager-side upload cap for early rejection.
const idolAvatarUploadLimitBytes int64 = 20 << 20

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
		// Keep locally uploaded avatars; they have no remote source to re-download.
		avatarManager.RemoveRemote(id)
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

	// Never let a browser or intermediary cache the option list; it changes as
	// soon as an avatar is uploaded or selected.
	c.Header("Cache-Control", "no-store")

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

	customAvatars, err := dbpkg.ListJavIdolAvatars(ctx, id)
	if err != nil {
		respondIdolAvatarLookupError(c, id, err)
		return
	}

	candidates := avatarManager.Candidates(ctx, names...)
	items := make([]gin.H, 0, len(candidates)+len(customAvatars)+1)
	selectedListed := false

	// The custom collection (uploaded files and manual URLs) persists even when
	// another avatar is currently selected.
	for _, customURL := range customAvatars {
		selected := selectedURL != "" && customURL == selectedURL
		if selected {
			selectedListed = true
		}
		_, exists := avatarManager.LocalPath(id, customURL)
		kind := "url"
		if manager.IsUploadAvatarURL(customURL) {
			kind = "upload"
		}
		items = append(items, gin.H{
			"key":      manager.CandidateKey(customURL),
			"source":   "custom",
			"kind":     kind,
			"selected": selected,
			"exists":   exists,
		})
	}

	for _, image := range candidates {
		key := manager.CandidateKey(image.URL)
		_, exists := avatarManager.LocalPath(id, image.URL)
		selected := selectedURL != "" && image.URL == selectedURL
		if selected {
			selectedListed = true
		}
		items = append(items, gin.H{
			"key":      key,
			"source":   image.Source,
			"selected": selected,
			"exists":   exists,
		})
	}

	// Keep a manual image URL (or a missing upload) visible as a custom selection.
	if selectedURL != "" && !selectedListed {
		_, exists := avatarManager.LocalPath(id, selectedURL)
		kind := "url"
		if manager.IsUploadAvatarURL(selectedURL) {
			kind = "upload"
		}
		custom := gin.H{
			"key":      manager.CandidateKey(selectedURL),
			"source":   "custom",
			"kind":     kind,
			"selected": true,
			"exists":   exists,
		}
		items = append([]gin.H{custom}, items...)
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
	candidateURL := ""
	if image, found := avatarManager.FindCandidate(ctx, key, names...); found {
		candidateURL = image.URL
	} else {
		// Uploaded photos and manual URLs are not Gfriends candidates.
		customAvatars, listErr := dbpkg.ListJavIdolAvatars(ctx, id)
		if listErr != nil {
			respondIdolAvatarLookupError(c, id, listErr)
			return
		}
		for _, customURL := range customAvatars {
			if manager.CandidateKey(customURL) == key {
				candidateURL = customURL
				break
			}
		}
		if candidateURL == "" {
			selectedURL, urlErr := dbpkg.GetJavIdolAvatarURL(ctx, id)
			if urlErr != nil {
				respondIdolAvatarLookupError(c, id, urlErr)
				return
			}
			if selectedURL != "" && manager.CandidateKey(selectedURL) == key {
				candidateURL = selectedURL
			}
		}
	}
	if candidateURL == "" {
		respondLocalizedError(c, http.StatusNotFound, "头像不存在", "Avatar was not found")
		return
	}
	avatarPath, err := avatarManager.Ensure(ctx, id, candidateURL)
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
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "选择头像请求无效", "Invalid avatar selection request")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), idolAvatarDownloadTimeout)
	defer cancel()

	// A manual image URL is downloaded, added to the collection and selected.
	if customURL := strings.TrimSpace(req.URL); customURL != "" {
		if _, err := avatarManager.Ensure(ctx, id, customURL); err != nil {
			logging.Info("idol avatar: custom url unavailable id=%d: %v", id, err)
			respondLocalizedError(c, http.StatusBadRequest, "下载图片失败，请检查图片地址", "Failed to download the image; check the image URL")
			return
		}
		if err := dbpkg.AddJavIdolAvatar(ctx, id, customURL); err != nil {
			respondJavIdolAvatarSaveError(c, id, err)
			return
		}
		if err := dbpkg.UpdateJavIdolAvatarURL(ctx, id, customURL); err != nil {
			_ = dbpkg.DeleteJavIdolAvatar(ctx, id, customURL)
			respondJavIdolAvatarSaveError(c, id, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"key": manager.CandidateKey(customURL), "url": customURL})
		return
	}

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
		// The key may select a custom avatar (upload or manual URL) from the collection.
		customAvatars, listErr := dbpkg.ListJavIdolAvatars(ctx, id)
		if listErr != nil {
			respondIdolAvatarLookupError(c, id, listErr)
			return
		}
		for _, customURL := range customAvatars {
			if manager.CandidateKey(customURL) != key {
				continue
			}
			if err := dbpkg.UpdateJavIdolAvatarURL(ctx, id, customURL); err != nil {
				respondJavIdolAvatarSaveError(c, id, err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"key": key})
			return
		}
		// Fall back to a legacy selection that predates the collection table.
		if selectedURL, urlErr := dbpkg.GetJavIdolAvatarURL(ctx, id); urlErr == nil &&
			selectedURL != "" && manager.CandidateKey(selectedURL) == key {
			c.JSON(http.StatusOK, gin.H{"key": key})
			return
		}
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

// uploadJavIdolAvatar stores a user-uploaded image and selects it for the idol.
func uploadJavIdolAvatar(c *gin.Context) {
	id, ok := parseJavIdolID(c)
	if !ok {
		return
	}
	avatarManager, ok := requireIdolAvatarManager(c)
	if !ok {
		return
	}

	if c.Request.ContentLength > idolAvatarUploadLimitBytes+(1<<20) {
		respondLocalizedError(c, http.StatusRequestEntityTooLarge, "图片文件过大", "The image file is too large")
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		respondLocalizedError(c, http.StatusBadRequest, "请选择要上传的图片", "Please choose an image to upload")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		logging.Error("open idol avatar upload id=%d: %v", id, err)
		respondLocalizedError(c, http.StatusInternalServerError, "读取上传图片失败", "Failed to read the uploaded image")
		return
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(c.Request.Context(), idolAvatarDownloadTimeout)
	defer cancel()

	uploadURL, err := avatarManager.SaveUpload(id, filepath.Ext(fileHeader.Filename), file)
	if err != nil {
		switch {
		case errors.Is(err, manager.ErrIdolAvatarUploadTooLarge):
			respondLocalizedError(c, http.StatusRequestEntityTooLarge, "图片文件过大", "The image file is too large")
		case errors.Is(err, manager.ErrIdolAvatarUploadType), errors.Is(err, manager.ErrIdolAvatarUploadInvalid):
			respondLocalizedError(c, http.StatusUnsupportedMediaType, "仅支持 JPG/PNG 图片", "Only JPG/PNG images are supported")
		default:
			logging.Error("save idol avatar upload id=%d: %v", id, err)
			respondLocalizedError(c, http.StatusInternalServerError, "保存上传图片失败", "Failed to save the uploaded image")
		}
		return
	}
	if err := dbpkg.AddJavIdolAvatar(ctx, id, uploadURL); err != nil {
		avatarManager.DeleteUpload(uploadURL)
		respondJavIdolAvatarSaveError(c, id, err)
		return
	}
	if err := dbpkg.UpdateJavIdolAvatarURL(ctx, id, uploadURL); err != nil {
		_ = dbpkg.DeleteJavIdolAvatar(ctx, id, uploadURL)
		avatarManager.DeleteUpload(uploadURL)
		respondJavIdolAvatarSaveError(c, id, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": manager.CandidateKey(uploadURL), "url": uploadURL})
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
