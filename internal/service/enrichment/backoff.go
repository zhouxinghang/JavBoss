package enrichment

import (
	"strconv"
	"sync"
	"time"
)

// Job names identify the periodic enrichment job an item belongs to, so the
// same row can back off independently for series, studio, and idol lookups.
const (
	enrichJobCensoredSeries   = "censored-jav-series"
	enrichJobUncensoredSeries = "uncensored-jav-series"
	enrichJobCensoredIdols    = "censored-jav-idols"
	enrichJobUncensoredIdols  = "uncensored-jav-idols"
	enrichJobCensoredStudio   = "censored-jav-studio"
	enrichJobUncensoredStudio = "uncensored-jav-studio"
	enrichJobIdolProfile      = "idol-profile"
)

const (
	backoffInitialDelay = 5 * time.Minute
	backoffMaxDelay     = 6 * time.Hour
)

// failureBackoff remembers how many times a work item failed in a row and the
// earliest time it may be retried. It keeps the periodic enrichment jobs from
// hammering providers (and the log) for items whose lookup keeps failing, for
// example a code that matches several movies and is therefore ambiguous.
//
// State is in-memory and intentionally resets on process restart: a single
// immediate retry after a restart is harmless, and it avoids a database
// migration for what is only a scheduling concern.
type failureBackoff struct {
	mu      sync.Mutex
	entries map[string]backoffEntry
}

type backoffEntry struct {
	failures int
	next     time.Time
}

func newFailureBackoff() *failureBackoff {
	return &failureBackoff{entries: make(map[string]backoffEntry)}
}

// allow reports whether the item is due for another attempt at now.
func (b *failureBackoff) allow(key string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	entry, ok := b.entries[key]
	if !ok {
		return true
	}
	return !now.Before(entry.next)
}

// succeed forgets the item's failure history.
func (b *failureBackoff) succeed(key string) {
	b.mu.Lock()
	delete(b.entries, key)
	b.mu.Unlock()
}

// fail records another failure and schedules the next attempt.
func (b *failureBackoff) fail(key string, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entry := b.entries[key]
	entry.failures++
	entry.next = now.Add(failureBackoffDelay(entry.failures))
	b.entries[key] = entry
}

// reset clears all recorded failures. It exists so tests start from a clean slate.
func (b *failureBackoff) reset() {
	b.mu.Lock()
	b.entries = make(map[string]backoffEntry)
	b.mu.Unlock()
}

// failureBackoffDelay returns the exponential delay for the nth consecutive failure.
func failureBackoffDelay(failures int) time.Duration {
	if failures <= 0 {
		return 0
	}
	delay := backoffInitialDelay
	for i := 1; i < failures; i++ {
		if delay >= backoffMaxDelay/2 {
			return backoffMaxDelay
		}
		delay *= 2
	}
	if delay > backoffMaxDelay {
		return backoffMaxDelay
	}
	return delay
}

var enrichmentBackoff = newFailureBackoff()

func enrichmentBackoffKey(job string, id int64) string {
	return job + ":" + strconv.FormatInt(id, 10)
}

// enrichmentBackoffAllow reports whether the job may retry the item now.
func enrichmentBackoffAllow(job string, id int64) bool {
	return enrichmentBackoff.allow(enrichmentBackoffKey(job, id), time.Now())
}

// enrichmentBackoffSucceed clears the item's failure history.
func enrichmentBackoffSucceed(job string, id int64) {
	enrichmentBackoff.succeed(enrichmentBackoffKey(job, id))
}

// enrichmentBackoffFail records a failed attempt and delays the next retry.
func enrichmentBackoffFail(job string, id int64) {
	enrichmentBackoff.fail(enrichmentBackoffKey(job, id), time.Now())
}
