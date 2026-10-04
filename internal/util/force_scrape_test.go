package util

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestForceScrapeContextMarker(t *testing.T) {
	if IsForceScrape(context.Background()) {
		t.Fatal("plain context should not be marked as forced")
	}
	if IsForceScrape(nil) {
		t.Fatal("nil context should not be marked as forced")
	}
	if !IsForceScrape(WithForceScrape(context.Background())) {
		t.Fatal("WithForceScrape context should be marked as forced")
	}
}

func TestForceScrapeBypassesNegativeURLCache(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	t.Cleanup(func() { negativeURLCache.Delete(server.URL) })

	client := WithNegativeCache(server.Client())
	request := func(ctx context.Context) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}

	request(context.Background())
	request(context.Background())
	if got := calls.Load(); got != 1 {
		t.Fatalf("network calls after cached miss=%d want=1", got)
	}

	request(WithForceScrape(context.Background()))
	if got := calls.Load(); got != 2 {
		t.Fatalf("network calls after forced miss=%d want=2", got)
	}
}
