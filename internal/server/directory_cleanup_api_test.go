package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"javboss/internal/common"
	dbpkg "javboss/internal/db"
	"javboss/internal/models"
	"javboss/internal/service"
)

func setupCleanupAPITest(t *testing.T) (*gin.Engine, models.Directory, string) {
	t.Helper()
	database, err := dbpkg.Open(filepath.Join(t.TempDir(), "cleanup-api.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	prevDB := common.DB
	common.DB = database
	t.Cleanup(func() {
		common.DB = prevDB
		if sqlDB, dbErr := database.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	root := t.TempDir()
	dir := models.Directory{Path: root}
	if err := database.Create(&dir).Error; err != nil {
		t.Fatalf("create directory: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/directories/:id/cleanup/scan", scanDirectoryCleanup)
	router.POST("/directories/:id/cleanup/delete", deleteDirectoryCleanupFiles)
	router.GET("/directories/:id/cleanup/preview", previewDirectoryCleanupFile)
	return router, dir, root
}

func makePNGBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{G: 200, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestDirectoryCleanupAPIScanPreviewAndDelete(t *testing.T) {
	router, dir, root := setupCleanupAPITest(t)

	writeFile := func(rel string, data []byte) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	writeFile(".DS_Store", []byte("ds"))
	writeFile("sub/最新地址.txt", []byte("ad"))
	writeFile("sub/扫码推广.png", makePNGBytes(t, 400, 400))
	writeFile("a/IPX-001.mp4", make([]byte, 1024))
	writeFile("b/IPX-001-C.mkv", make([]byte, 2048))

	baseURL := "/directories/" + strconv.FormatInt(dir.ID, 10) + "/cleanup"

	// 1. Scan all categories + duplicates
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, baseURL+"/scan", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("scan status=%d body=%s", rec.Code, rec.Body.String())
	}
	var scanRes service.DirectoryCleanupScanResult
	if err := json.Unmarshal(rec.Body.Bytes(), &scanRes); err != nil {
		t.Fatalf("decode scan response: %v", err)
	}
	if scanRes.JunkTotalCount != 3 || scanRes.DuplicateGroupCount != 1 {
		t.Fatalf("unexpected scan summary: junk=%d dups=%d body=%s", scanRes.JunkTotalCount, scanRes.DuplicateGroupCount, rec.Body.String())
	}

	// 2. Reject invalid category
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, baseURL+"/scan", strings.NewReader(`{"categories":["nope"]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid category, got %d", rec.Code)
	}

	// 3. Preview suspected ad image
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, baseURL+"/preview?path="+url.QueryEscape("sub/扫码推广.png"), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/png") {
		t.Fatalf("preview status=%d content-type=%q", rec.Code, rec.Header().Get("Content-Type"))
	}

	// 4. Reject non-image preview and path traversal preview
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, baseURL+"/preview?path="+url.QueryEscape("sub/最新地址.txt"), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-image preview, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, baseURL+"/preview?path="+url.QueryEscape("../outside.png"), nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for traversal preview, got %d", rec.Code)
	}

	// 5. Delete selected junk + duplicate video
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodPost,
		baseURL+"/delete",
		strings.NewReader(`{"paths":["sub/最新地址.txt","sub/扫码推广.png","a/IPX-001.mp4"],"remove_empty_dirs":true}`),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", rec.Code, rec.Body.String())
	}
	var delRes service.DirectoryCleanupDeleteResult
	if err := json.Unmarshal(rec.Body.Bytes(), &delRes); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if delRes.DeletedCount != 3 || delRes.EmptyDirectoriesMoved != 2 || len(delRes.Failed) != 0 {
		t.Fatalf("unexpected delete result: %+v", delRes)
	}
	if _, err := os.Stat(filepath.Join(root, "b", "IPX-001-C.mkv")); err != nil {
		t.Fatalf("unselected video should still exist: %v", err)
	}
}
