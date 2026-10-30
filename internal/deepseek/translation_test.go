package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func translationResponse(content, finish string) string {
	data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
		"message": map[string]string{"content": content}, "finish_reason": finish,
	}}})
	return string(data)
}

func TestTranslateTitle(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"success", translationResponse(`{"zh_title":" 宁静的午后 "}`, "stop"), 200, "宁静的午后"},
		{"authentication", `{"error":"secret-key"}`, 401, ""},
		{"rate limit", `{}`, 429, ""},
		{"empty choices", `{"choices":[]}`, 200, ""},
		{"malformed response", `{`, 200, ""},
		{"truncated", translationResponse(`{"zh_title":"中文"}`, "length"), 200, ""},
		{"refused", translationResponse(`{"zh_title":"中文"}`, "content_filter"), 200, ""},
		{"unstructured", translationResponse("中文", "stop"), 200, ""},
		{"empty translation", translationResponse(`{"zh_title":" "}`, "stop"), 200, ""},
		{"untranslated", translationResponse(`{"zh_title":"A quiet afternoon"}`, "stop"), 200, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost || req.URL.String() != "https://api.deepseek.com/chat/completions" ||
					req.Header.Get("Authorization") != "Bearer secret-key" || req.Header.Get("Content-Type") != "application/json" {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
				}
				var payload struct {
					Model    string                           `json:"model"`
					Messages []struct{ Role, Content string } `json:"messages"`
					Thinking map[string]string                `json:"thinking"`
				}
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if payload.Model != DefaultModel || len(payload.Messages) != 2 || payload.Messages[1].Content != "A quiet afternoon" || payload.Thinking["type"] != "disabled" {
					t.Fatalf("unexpected translation payload: %+v", payload)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			got, err := TranslateTitle(context.Background(), client, Settings{APIKey: "secret-key"}, " A quiet afternoon ")
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("title=%q err=%v", got, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret-key") {
				t.Fatal("error exposed API key")
			}
		})
	}
}

func TestTranslateTitleValidationAndCancellation(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	})}
	for _, tc := range []struct {
		title, key string
		want       error
	}{
		{" ", "key", ErrInvalidTitle},
		{strings.Repeat("文", MaxTitleLength+1), "key", ErrInvalidTitle},
		{"title", "", ErrNotConfigured},
	} {
		_, err := TranslateTitle(context.Background(), client, Settings{APIKey: tc.key}, tc.title)
		if !errors.Is(err, tc.want) {
			t.Fatalf("err=%v, want %v", err, tc.want)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := TranslateTitle(ctx, client, Settings{APIKey: "key"}, "Title"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error=%v", err)
	}
}
