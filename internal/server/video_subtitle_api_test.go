package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"javboss/internal/common"
	dbpkg "javboss/internal/db"
	"javboss/internal/models"
	"javboss/internal/subtitle"
)

func TestVideoSubtitleUploadLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := dbpkg.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	previousDB, previousConfig := common.DB, common.AppConfig
	common.DB = database
	common.AppConfig = &common.Config{DatabasePath: dbPath}
	t.Cleanup(func() {
		common.DB, common.AppConfig = previousDB, previousConfig
		sqlDB, _ := database.DB()
		_ = sqlDB.Close()
	})
	video := models.Video{Fingerprint: "subtitle-upload-lifecycle"}
	if err := database.Create(&video).Error; err != nil {
		t.Fatal(err)
	}
	directory := models.Directory{Path: t.TempDir()}
	if err := database.Create(&directory).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := dbpkg.UpsertVideoLocation(context.Background(), video.ID, directory.ID, "movie.mp4", time.Now()); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router)
	endpoint := fmt.Sprintf("/videos/%d/subtitle", video.ID)
	request := func(method, path, contentType string, body io.Reader, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, body)
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w
	}
	upload := func(path, name, content string, want int) {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(part, content)
		_ = writer.Close()
		request(http.MethodPut, path, writer.FormDataContentType(), &body, want)
	}
	request(http.MethodGet, endpoint+"/file", "", nil, http.StatusNotFound)
	const content = "1\n00:00:00,000 --> 00:00:10,000\n字幕测试\n"
	upload(endpoint, "上传的字幕.srt", content, http.StatusOK)
	response := request(http.MethodGet, endpoint, "", nil, http.StatusOK)
	var metadata struct {
		Subtitle subtitle.File `json:"subtitle"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Subtitle.Name != "上传的字幕.srt" || metadata.Subtitle.Path != "" {
		t.Fatalf("metadata = %#v", metadata)
	}
	upload(endpoint, "invalid.srt", "invalid", http.StatusBadRequest)
	response = request(http.MethodGet, endpoint+"/file", "", nil, http.StatusOK)
	if response.Body.String() != content {
		t.Fatalf("download = %q", response.Body.String())
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff header")
	}
	upload("/videos/999999/subtitle", "valid.srt", content, http.StatusNotFound)
	request(http.MethodGet, "/videos/invalid/subtitle", "", nil, http.StatusBadRequest)
	upload(endpoint, "large.srt", string(bytes.Repeat([]byte{'a'}, int(subtitle.MaxSize)+1)), http.StatusRequestEntityTooLarge)
	request(http.MethodDelete, endpoint, "", nil, http.StatusOK)
	request(http.MethodGet, endpoint+"/file", "", nil, http.StatusNotFound)
	response = request(http.MethodGet, endpoint, "", nil, http.StatusOK)
	if response.Body.String() != `{"subtitle":null}` {
		t.Fatalf("deleted metadata = %s", response.Body.String())
	}
}
