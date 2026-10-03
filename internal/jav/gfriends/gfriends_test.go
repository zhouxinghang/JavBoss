package gfriends

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseFileTreeStripsAIFixAndBuildsURLs(t *testing.T) {
	raw := []byte(`{
	  "Information": {"TotalNum": 3},
	  "Content": {
	    "1": {
	      "AI-Fix-三上悠亜.jpg": "abc.jpg",
	      "葵つかさ.jpg": "def.png",
	      "みさと.jpg": "みさと.jpg?t=1658331538"
	    }
	  }
	}`)

	tree, err := parseFileTree(raw, "https://example.com/repo/")
	if err != nil {
		t.Fatalf("parseFileTree: %v", err)
	}
	if got := tree["三上悠亜"]; len(got) != 1 || got[0].URL != "https://example.com/repo/Content/1/abc.jpg" || got[0].Source != "1" {
		t.Fatalf("unexpected AI-Fix entry: %#v", got)
	}
	if got := tree["葵つかさ"]; len(got) != 1 || got[0].URL != "https://example.com/repo/Content/1/def.png" {
		t.Fatalf("unexpected plain entry: %#v", got)
	}
	// The cache-busting query token must stay a query, not be escaped into the path.
	if got := tree["みさと"]; len(got) != 1 || got[0].URL != "https://example.com/repo/Content/1/%E3%81%BF%E3%81%95%E3%81%A8.jpg?t=1658331538" {
		t.Fatalf("unexpected query-token entry: %#v", got)
	}
	if _, ok := tree["AI-Fix-三上悠亜"]; ok {
		t.Fatal("AI-Fix prefix should be stripped from the lookup name")
	}
}

func TestNameCandidatesStripsParenthesizedSuffix(t *testing.T) {
	cases := map[string]string{
		"あいうえお（かな）": "あいうえお",
		"あいうえお(かな)": "あいうえお",
		"そのまま":      "そのまま",
	}
	for input, want := range cases {
		candidates := nameCandidates(input)
		if candidates[0] != input {
			t.Fatalf("first candidate for %q = %q, want the exact name", input, candidates[0])
		}
		if len(candidates) < 2 && input != want {
			t.Fatalf("expected a stripped candidate for %q, got %#v", input, candidates)
		}
		if len(candidates) >= 2 && candidates[1] != want {
			t.Fatalf("stripped candidate for %q = %q, want %q", input, candidates[1], want)
		}
	}
}

func TestLookupFallsBackToStrippedName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Filetree.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"Content":{"2":{"三上悠亜.jpg":"x.jpg"}}}`))
	}))
	defer server.Close()

	client := New(WithRepositoryURL(server.URL), WithHTTPClient(server.Client()))
	images := client.Lookup(context.Background(), "三上悠亜（旧名）")
	if len(images) != 1 {
		t.Fatalf("Lookup = %#v, want one image", images)
	}
}

func TestLoadTreeUsesDiskCacheWhenFetchFails(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "Filetree.json")
	cached := []byte(`{"Content":{"3":{"葵つかさ.jpg":"y.jpg"}}}`)
	if err := os.WriteFile(cachePath, cached, 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	client := New(
		WithRepositoryURL("http://127.0.0.1:0/"),
		WithCachePath(cachePath),
		WithHTTPClient(&http.Client{Timeout: 200 * time.Millisecond}),
	)
	images := client.Lookup(context.Background(), "葵つかさ")
	if len(images) != 1 || images[0].URL != "http://127.0.0.1:0/Content/3/y.jpg" {
		t.Fatalf("Lookup from cache = %#v", images)
	}
}
