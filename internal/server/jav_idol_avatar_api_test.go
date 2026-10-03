package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"javboss/internal/common"
	dbpkg "javboss/internal/db"
	"javboss/internal/jav/gfriends"
	"javboss/internal/manager"
	"javboss/internal/models"
)

func avatarTestJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 11)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func setupJavIdolAvatarTest(t *testing.T) (*gin.Engine, *models.JavIdol) {
	t.Helper()
	imageBytes := avatarTestJPEG(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/Filetree.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Content":{"1":{"三上悠亜.jpg":"a.jpg"},"2":{"三上悠亜.jpg":"AI-Fix-b.jpg"}}}`))
	})
	mux.HandleFunc("/Content/1/a.jpg", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(imageBytes)
	})
	mux.HandleFunc("/Content/2/AI-Fix-b.jpg", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(imageBytes)
	})
	repo := httptest.NewServer(mux)
	t.Cleanup(repo.Close)

	avatarDir := t.TempDir()
	database, err := dbpkg.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	previousDB := common.DB
	previousConfig := common.AppConfig
	previousManager := common.IdolAvatarManager
	common.DB = database
	common.AppConfig = &common.Config{JavIdolDir: avatarDir}
	common.IdolAvatarManager = manager.NewIdolAvatarManager(avatarDir, gfriends.New(
		gfriends.WithRepositoryURL(repo.URL),
		gfriends.WithHTTPClient(repo.Client()),
		gfriends.WithCachePath(filepath.Join(avatarDir, "Filetree.json")),
	))
	t.Cleanup(func() {
		common.DB = previousDB
		common.AppConfig = previousConfig
		common.IdolAvatarManager = previousManager
		if sqlDB, dbErr := database.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	idol := models.JavIdol{Name: "三上悠亜", JapaneseName: "三上悠亜"}
	if err := database.Create(&idol).Error; err != nil {
		t.Fatalf("create idol: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/jav/idols/:id/avatar", getJavIdolAvatar)
	router.PUT("/jav/idols/:id/avatar", updateJavIdolAvatar)
	router.GET("/jav/idols/:id/avatar/options", listJavIdolAvatarOptions)
	router.GET("/jav/idols/:id/avatar/candidates/:key", getJavIdolAvatarCandidate)
	return router, &idol
}

func doJavIdolAvatarRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestJavIdolAvatarPickerFlow(t *testing.T) {
	router, idol := setupJavIdolAvatarTest(t)
	base := "/jav/idols/" + strconv.FormatInt(idol.ID, 10)

	optionsResp := doJavIdolAvatarRequest(router, http.MethodGet, base+"/avatar/options", "")
	if optionsResp.Code != http.StatusOK {
		t.Fatalf("options status = %d body=%s", optionsResp.Code, optionsResp.Body.String())
	}
	var options struct {
		AutoSelected bool `json:"auto_selected"`
		Items        []struct {
			Key      string `json:"key"`
			Source   string `json:"source"`
			Selected bool   `json:"selected"`
		} `json:"items"`
	}
	if err := json.Unmarshal(optionsResp.Body.Bytes(), &options); err != nil {
		t.Fatalf("decode options: %v", err)
	}
	if len(options.Items) != 2 {
		t.Fatalf("options items = %#v, want 2", options.Items)
	}
	if !options.AutoSelected {
		t.Fatal("auto should be selected before any explicit choice")
	}
	// AI-Fix candidates are preferred, so the first item must come from shard 2.
	if options.Items[0].Source != "2" {
		t.Fatalf("first candidate source = %q, want AI-Fix shard 2", options.Items[0].Source)
	}
	chosenKey := options.Items[1].Key
	if chosenKey == "" || options.Items[0].Key == "" {
		t.Fatal("candidate keys must not be empty")
	}

	autoResp := doJavIdolAvatarRequest(router, http.MethodGet, base+"/avatar", "")
	if autoResp.Code != http.StatusOK {
		t.Fatalf("auto avatar status = %d body=%s", autoResp.Code, autoResp.Body.String())
	}

	candidateResp := doJavIdolAvatarRequest(router, http.MethodGet, base+"/avatar/candidates/"+chosenKey, "")
	if candidateResp.Code != http.StatusOK {
		t.Fatalf("candidate status = %d body=%s", candidateResp.Code, candidateResp.Body.String())
	}

	selectResp := doJavIdolAvatarRequest(router, http.MethodPut, base+"/avatar", `{"key":"`+chosenKey+`"}`)
	if selectResp.Code != http.StatusOK {
		t.Fatalf("select status = %d body=%s", selectResp.Code, selectResp.Body.String())
	}

	selectedResp := doJavIdolAvatarRequest(router, http.MethodGet, base+"/avatar/options", "")
	var selected struct {
		AutoSelected bool `json:"auto_selected"`
		Items        []struct {
			Key      string `json:"key"`
			Selected bool   `json:"selected"`
		} `json:"items"`
	}
	if err := json.Unmarshal(selectedResp.Body.Bytes(), &selected); err != nil {
		t.Fatalf("decode selected options: %v", err)
	}
	if selected.AutoSelected {
		t.Fatal("auto must not be selected after choosing a candidate")
	}
	var selectedCount int
	for _, item := range selected.Items {
		if item.Selected {
			selectedCount++
			if item.Key != chosenKey {
				t.Fatalf("selected key = %q, want %q", item.Key, chosenKey)
			}
		}
	}
	if selectedCount != 1 {
		t.Fatalf("selected count = %d, want 1", selectedCount)
	}

	resetResp := doJavIdolAvatarRequest(router, http.MethodPut, base+"/avatar", `{"key":""}`)
	if resetResp.Code != http.StatusOK {
		t.Fatalf("reset status = %d body=%s", resetResp.Code, resetResp.Body.String())
	}
	if avatarURL, err := dbpkg.GetJavIdolAvatarURL(t.Context(), idol.ID); err != nil || avatarURL != "" {
		t.Fatalf("avatar url after reset = %q err=%v", avatarURL, err)
	}
}

func TestJavIdolAvatarRejectsInvalidID(t *testing.T) {
	router, _ := setupJavIdolAvatarTest(t)
	recorder := doJavIdolAvatarRequest(router, http.MethodGet, "/jav/idols/0/avatar", "")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestJavIdolAvatarRejectsUnknownCandidateKey(t *testing.T) {
	router, idol := setupJavIdolAvatarTest(t)
	base := "/jav/idols/" + strconv.FormatInt(idol.ID, 10)
	recorder := doJavIdolAvatarRequest(router, http.MethodPut, base+"/avatar", `{"key":"deadbeefdeadbeef"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", recorder.Code, recorder.Body.String())
	}
}
