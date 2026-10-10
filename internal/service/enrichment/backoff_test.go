package enrichment

import (
	"context"
	"errors"
	"testing"
	"time"

	"javboss/internal/jav"
	"javboss/internal/models"
)

func TestFailureBackoffDelayGrowsAndCaps(t *testing.T) {
	want := []time.Duration{
		5 * time.Minute,
		10 * time.Minute,
		20 * time.Minute,
		40 * time.Minute,
		80 * time.Minute,
		160 * time.Minute,
		320 * time.Minute,
		6 * time.Hour,
		6 * time.Hour,
	}
	for i, expected := range want {
		if got := failureBackoffDelay(i + 1); got != expected {
			t.Fatalf("failureBackoffDelay(%d)=%s, want %s", i+1, got, expected)
		}
	}
	if got := failureBackoffDelay(0); got != 0 {
		t.Fatalf("failureBackoffDelay(0)=%s, want 0", got)
	}
}

func TestFailureBackoffWindowAndReset(t *testing.T) {
	backoff := newFailureBackoff()
	now := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)

	if !backoff.allow("k", now) {
		t.Fatal("new key should be allowed")
	}
	backoff.fail("k", now)
	if backoff.allow("k", now.Add(time.Minute)) {
		t.Fatal("key should be blocked before its next attempt")
	}
	if !backoff.allow("k", now.Add(5*time.Minute)) {
		t.Fatal("key should be allowed once the window elapses")
	}
	backoff.fail("k", now.Add(5*time.Minute))
	if backoff.allow("k", now.Add(5*time.Minute+10*time.Minute-time.Second)) {
		t.Fatal("second failure should double the window")
	}
	backoff.succeed("k")
	if !backoff.allow("k", now) {
		t.Fatal("success should clear the failure history")
	}
}

func TestCensoredSeriesBackoffSkipsPersistentHardFailures(t *testing.T) {
	gdb := openEnrichmentTestDB(t)
	row := models.Jav{Code: "AMBIGUOUS-001"}
	if err := gdb.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	var calls int
	lookup := func(context.Context, string, jav.Provider) (*jav.JavInfo, error) {
		calls++
		return nil, errors.New("ambiguous code")
	}
	if err := enrichCensoredSeries(context.Background(), lookup); err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatal("expected provider lookups on the first attempt")
	}
	calls = 0
	if err := enrichCensoredSeries(context.Background(), lookup); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("persistently failing item was retried despite backoff: calls=%d", calls)
	}
}

func TestCensoredSeriesMissesAreNotBackedOff(t *testing.T) {
	gdb := openEnrichmentTestDB(t)
	row := models.Jav{Code: "MISSING-001"}
	if err := gdb.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	var calls int
	lookup := func(context.Context, string, jav.Provider) (*jav.JavInfo, error) {
		calls++
		return nil, jav.ErrNotFound
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := enrichCensoredSeries(context.Background(), lookup); err != nil {
			t.Fatal(err)
		}
	}
	if calls == 0 {
		t.Fatal("definite misses should keep being retried")
	}
}
