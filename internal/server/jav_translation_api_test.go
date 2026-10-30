package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"javboss/internal/common"
	dbpkg "javboss/internal/db"
	"javboss/internal/models"
)

func TestDeepSeekConfigKeepsKeyPrivate(t *testing.T) {
	testAuthService(t)
	router := gin.New()
	RegisterRoutes(router)
	request := func(method, path, body string, status int) map[string]string {
		t.Helper()
		res := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("status=%d body=%s", res.Code, res.Body)
		}
		if strings.Contains(res.Body.String(), "private-test-key") {
			t.Fatal("config leaked API key")
		}
		var result map[string]string
		if status == 200 {
			if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	request("POST", "/jav/title-translations", `{"title":"Original"}`, 400)
	request("POST", "/jav/title-translations", `{"title":" "}`, 400)
	request("PATCH", "/config", `{"deepseek_api_key":"invalid\nkey"}`, 400)
	request("PATCH", "/config", `{"deepseek_model":"invalid model"}`, 400)
	cfg := request("PATCH", "/config", `{"deepseek_api_key":" private-test-key ","deepseek_model":"deepseek-flash","deepseek_auto_translate":true}`, 200)
	if _, ok := cfg["deepseek_api_key"]; ok || cfg["deepseek_api_key_configured"] != "true" || cfg["deepseek_auto_translate"] != "true" {
		t.Fatalf("config=%v", cfg)
	}
	request("GET", "/config", "", 200)
	request("PATCH", "/config", `{"deepseek_model":"deepseek-v4-pro"}`, 200)
	stored, err := dbpkg.ListConfig(context.Background())
	if err != nil || stored["deepseek_api_key"] != "private-test-key" {
		t.Fatal("omitted key should preserve saved key")
	}
	cfg = request("PATCH", "/config", `{"deepseek_api_key":"","deepseek_auto_translate":false}`, 200)
	if cfg["deepseek_api_key_configured"] != "false" || cfg["deepseek_auto_translate"] != "false" {
		t.Fatalf("config=%v", cfg)
	}
}

func TestEditChineseTitleKeepsOriginal(t *testing.T) {
	testAuthService(t)
	item := models.Jav{Code: "ABC-001", Title: "Original title"}
	if err := common.DB.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterRoutes(router)
	for _, tc := range []struct{ payload, want string }{
		{`{"zh_title":" 中文标题 "}`, "中文标题"},
		{`{"duration_min":90}`, "中文标题"},
		{`{"zh_title":""}`, ""},
	} {
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/jav/items/%d", item.ID), strings.NewReader(tc.payload))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", res.Code, res.Body)
		}
		var got models.Jav
		if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Title != item.Title || got.ZhTitle != tc.want {
			t.Fatalf("titles=%q/%q", got.Title, got.ZhTitle)
		}
	}
}
