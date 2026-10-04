package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
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

func setupJavIdolAvatarTest(t *testing.T) (*gin.Engine, *models.JavIdol, string) {
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
	mux.HandleFunc("/custom/avatar.jpg", func(w http.ResponseWriter, r *http.Request) {
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
	router.POST("/jav/idols/:id/avatar/upload", uploadJavIdolAvatar)
	router.GET("/jav/idols/:id/avatar/options", listJavIdolAvatarOptions)
	router.GET("/jav/idols/:id/avatar/candidates/:key", getJavIdolAvatarCandidate)
	return router, &idol, repo.URL + "/custom/avatar.jpg"
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
	router, idol, _ := setupJavIdolAvatarTest(t)
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
	router, _, _ := setupJavIdolAvatarTest(t)
	recorder := doJavIdolAvatarRequest(router, http.MethodGet, "/jav/idols/0/avatar", "")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestJavIdolAvatarRejectsUnknownCandidateKey(t *testing.T) {
	router, idol, _ := setupJavIdolAvatarTest(t)
	base := "/jav/idols/" + strconv.FormatInt(idol.ID, 10)
	recorder := doJavIdolAvatarRequest(router, http.MethodPut, base+"/avatar", `{"key":"deadbeefdeadbeef"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", recorder.Code, recorder.Body.String())
	}
}

func TestJavIdolAvatarUploadFlow(t *testing.T) {
	router, idol, _ := setupJavIdolAvatarTest(t)
	base := "/jav/idols/" + strconv.FormatInt(idol.ID, 10)
	imageBytes := avatarTestJPEG(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "avatar.jpg")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(imageBytes); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, base+"/avatar/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Key string `json:"key"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if payload.Key == "" || !strings.HasPrefix(payload.URL, "upload:") {
		t.Fatalf("upload payload = %#v", payload)
	}
	if stored, err := dbpkg.GetJavIdolAvatarURL(t.Context(), idol.ID); err != nil || stored != payload.URL {
		t.Fatalf("stored avatar url = %q err=%v", stored, err)
	}

	avatarResp := doJavIdolAvatarRequest(router, http.MethodGet, base+"/avatar", "")
	if avatarResp.Code != http.StatusOK {
		t.Fatalf("avatar status = %d body=%s", avatarResp.Code, avatarResp.Body.String())
	}
	if !bytes.Equal(avatarResp.Body.Bytes(), imageBytes) {
		t.Fatal("served avatar does not match the uploaded image")
	}

	candidateResp := doJavIdolAvatarRequest(router, http.MethodGet, base+"/avatar/candidates/"+payload.Key, "")
	if candidateResp.Code != http.StatusOK {
		t.Fatalf("candidate status = %d body=%s", candidateResp.Code, candidateResp.Body.String())
	}

	assertSelectedOption(t, router, base, "upload")
}

func TestJavIdolAvatarUploadCollectionPersists(t *testing.T) {
	router, idol, _ := setupJavIdolAvatarTest(t)
	base := "/jav/idols/" + strconv.FormatInt(idol.ID, 10)

	firstKey, _ := uploadAvatarViaAPI(t, router, base, "first.jpg", avatarTestJPEG(t))
	secondKey, _ := uploadAvatarViaAPI(t, router, base, "second.jpg", avatarTestJPEG(t))

	// Both uploads stay in the collection and the newest is selected.
	items := listAvatarOptionItems(t, router, base)
	if countAvatarOptions(items, "upload") != 2 {
		t.Fatalf("upload options = %#v, want 2", items)
	}
	for _, item := range items {
		if item.Kind == "upload" && item.Selected && item.Key != secondKey {
			t.Fatalf("selected upload = %q, want newest %q", item.Key, secondKey)
		}
	}

	// Selecting a Gfriends candidate must not discard uploaded photos.
	optionsResp := doJavIdolAvatarRequest(router, http.MethodGet, base+"/avatar/options", "")
	var options struct {
		Items []struct {
			Key  string `json:"key"`
			Kind string `json:"kind"`
		} `json:"items"`
	}
	if err := json.Unmarshal(optionsResp.Body.Bytes(), &options); err != nil {
		t.Fatalf("decode options: %v", err)
	}
	candidateKey := ""
	for _, item := range options.Items {
		if item.Kind == "" {
			candidateKey = item.Key
			break
		}
	}
	if candidateKey == "" {
		t.Fatal("expected a Gfriends candidate option")
	}
	selectResp := doJavIdolAvatarRequest(router, http.MethodPut, base+"/avatar", `{"key":"`+candidateKey+`"}`)
	if selectResp.Code != http.StatusOK {
		t.Fatalf("select candidate status = %d body=%s", selectResp.Code, selectResp.Body.String())
	}

	items = listAvatarOptionItems(t, router, base)
	if countAvatarOptions(items, "upload") != 2 {
		t.Fatalf("uploads were discarded after switching avatar: %#v", items)
	}
	for _, item := range items {
		if item.Kind == "upload" && item.Selected {
			t.Fatalf("upload %q should not be selected anymore", item.Key)
		}
	}

	// Non-selected uploads remain servable through the candidate endpoint.
	for _, key := range []string{firstKey, secondKey} {
		resp := doJavIdolAvatarRequest(router, http.MethodGet, base+"/avatar/candidates/"+key, "")
		if resp.Code != http.StatusOK {
			t.Fatalf("candidate %s status = %d body=%s", key, resp.Code, resp.Body.String())
		}
	}
}

func TestJavIdolAvatarSetCustomURL(t *testing.T) {
	router, idol, customURL := setupJavIdolAvatarTest(t)
	base := "/jav/idols/" + strconv.FormatInt(idol.ID, 10)

	resp := doJavIdolAvatarRequest(router, http.MethodPut, base+"/avatar", `{"url":"`+customURL+`"}`)
	if resp.Code != http.StatusOK {
		t.Fatalf("set url status = %d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode set url response: %v", err)
	}
	if stored, err := dbpkg.GetJavIdolAvatarURL(t.Context(), idol.ID); err != nil || stored != customURL {
		t.Fatalf("stored avatar url = %q err=%v", stored, err)
	}

	options := assertSelectedOption(t, router, base, "url")
	if options != payload.Key {
		t.Fatalf("selected option key = %q, want %q", options, payload.Key)
	}

	// Selecting a Gfriends candidate must not discard the manual URL.
	candidateKey := ""
	for _, item := range listAvatarOptionItems(t, router, base) {
		if item.Kind == "" {
			candidateKey = item.Key
			break
		}
	}
	if candidateKey == "" {
		t.Fatal("expected a Gfriends candidate option")
	}
	selectResp := doJavIdolAvatarRequest(router, http.MethodPut, base+"/avatar", `{"key":"`+candidateKey+`"}`)
	if selectResp.Code != http.StatusOK {
		t.Fatalf("select candidate status = %d body=%s", selectResp.Code, selectResp.Body.String())
	}

	items := listAvatarOptionItems(t, router, base)
	if countAvatarOptions(items, "url") != 1 {
		t.Fatalf("manual URL was discarded after switching avatar: %#v", items)
	}
	for _, item := range items {
		if item.Kind == "url" {
			if item.Selected {
				t.Fatalf("manual URL %q should not be selected anymore", item.Key)
			}
			if item.Key != payload.Key {
				t.Fatalf("manual URL key = %q, want %q", item.Key, payload.Key)
			}
		}
	}

	// The manual URL remains selectable again from the collection.
	reselect := doJavIdolAvatarRequest(router, http.MethodPut, base+"/avatar", `{"key":"`+payload.Key+`"}`)
	if reselect.Code != http.StatusOK {
		t.Fatalf("reselect custom status = %d body=%s", reselect.Code, reselect.Body.String())
	}
	if stored, err := dbpkg.GetJavIdolAvatarURL(t.Context(), idol.ID); err != nil || stored != customURL {
		t.Fatalf("stored avatar url = %q err=%v", stored, err)
	}
}

type avatarOptionItem struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	Selected bool   `json:"selected"`
}

func listAvatarOptionItems(t *testing.T, router http.Handler, base string) []avatarOptionItem {
	t.Helper()
	resp := doJavIdolAvatarRequest(router, http.MethodGet, base+"/avatar/options", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("options status = %d body=%s", resp.Code, resp.Body.String())
	}
	var options struct {
		Items []avatarOptionItem `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &options); err != nil {
		t.Fatalf("decode options: %v", err)
	}
	return options.Items
}

func countAvatarOptions(items []avatarOptionItem, kind string) int {
	count := 0
	for _, item := range items {
		if item.Kind == kind {
			count++
		}
	}
	return count
}

func uploadAvatarViaAPI(t *testing.T, router http.Handler, base, filename string, imageBytes []byte) (string, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(imageBytes); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, base+"/avatar/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload %s status = %d body=%s", filename, recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Key string `json:"key"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	return payload.Key, payload.URL
}

// assertSelectedOption checks that a single custom option of the given kind is
// selected and returns its key.
func assertSelectedOption(t *testing.T, router http.Handler, base, kind string) string {
	t.Helper()
	resp := doJavIdolAvatarRequest(router, http.MethodGet, base+"/avatar/options", "")
	if resp.Code != http.StatusOK {
		t.Fatalf("options status = %d body=%s", resp.Code, resp.Body.String())
	}
	var options struct {
		AutoSelected bool `json:"auto_selected"`
		Items        []struct {
			Key      string `json:"key"`
			Kind     string `json:"kind"`
			Selected bool   `json:"selected"`
		} `json:"items"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &options); err != nil {
		t.Fatalf("decode options: %v", err)
	}
	if options.AutoSelected {
		t.Fatal("auto must not be selected for a custom avatar")
	}
	var key string
	for _, item := range options.Items {
		if item.Kind == kind && item.Selected {
			key = item.Key
		}
	}
	if key == "" {
		t.Fatalf("no selected %q option in %#v", kind, options.Items)
	}
	return key
}
