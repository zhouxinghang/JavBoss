package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"javboss/internal/common"
	dbpkg "javboss/internal/db"
	"javboss/internal/manager"
	"javboss/internal/models"

	"github.com/gin-gonic/gin"
)

func TestUpdateJavCoverFromScreenshot(t *testing.T) {
	dataDir := t.TempDir()
	coverDir := filepath.Join(dataDir, "cover")
	dbPath := filepath.Join(dataDir, "javboss.db")
	database, err := dbpkg.Open(dbPath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	previousDB := common.DB
	common.DB = database
	previousConfig := common.AppConfig
	common.AppConfig = &common.Config{DatabasePath: dbPath, JavCoverDir: coverDir}
	t.Cleanup(func() {
		common.DB = previousDB
		common.AppConfig = previousConfig
		if sqlDB, dbErr := database.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	video := models.Video{Fingerprint: "jav-cover-screenshot", DurationSec: 1800}
	if err := database.Create(&video).Error; err != nil {
		t.Fatalf("create video: %v", err)
	}

	screenshotDir := filepath.Join(dataDir, "video", strconv.FormatInt(video.ID, 10), "screenshot")
	if err := os.MkdirAll(screenshotDir, 0o755); err != nil {
		t.Fatalf("mkdir screenshot dir: %v", err)
	}
	name := "mpv_00-01-23.jpg"
	screenshotPath := filepath.Join(screenshotDir, name)
	writeTestScreenshot(t, screenshotPath)
	want, err := os.ReadFile(screenshotPath)
	if err != nil {
		t.Fatalf("read screenshot: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/jav/:code/cover/screenshot", updateJavCoverFromScreenshot)
	body := fmt.Sprintf(`{"video_id": %d, "screenshot_name": %q}`, video.ID, name)
	req := httptest.NewRequest(http.MethodPut, "/jav/ABC-123/cover/screenshot", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("set JAV cover status = %d, body=%s", recorder.Code, recorder.Body.String())
	}

	coverPath, ok := manager.FindCoverPath(coverDir, "ABC-123")
	if !ok {
		t.Fatal("JAV cover file was not created")
	}
	got, err := os.ReadFile(coverPath)
	if err != nil {
		t.Fatalf("read JAV cover: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("JAV cover content does not match the screenshot")
	}
}

func TestUpdateJavCoverFromScreenshotMissingFile(t *testing.T) {
	dataDir := t.TempDir()
	coverDir := filepath.Join(dataDir, "cover")
	dbPath := filepath.Join(dataDir, "javboss.db")
	database, err := dbpkg.Open(dbPath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	previousDB := common.DB
	common.DB = database
	previousConfig := common.AppConfig
	common.AppConfig = &common.Config{DatabasePath: dbPath, JavCoverDir: coverDir}
	t.Cleanup(func() {
		common.DB = previousDB
		common.AppConfig = previousConfig
		if sqlDB, dbErr := database.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/jav/:code/cover/screenshot", updateJavCoverFromScreenshot)
	body := `{"video_id": 9, "screenshot_name": "mpv_00-00-01.jpg"}`
	req := httptest.NewRequest(http.MethodPut, "/jav/ABC-123/cover/screenshot", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing screenshot status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if _, ok := manager.FindCoverPath(coverDir, "ABC-123"); ok {
		t.Fatal("JAV cover should not be created for a missing screenshot")
	}
}
