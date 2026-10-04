package util

import "context"

type forceScrapeContextKey struct{}

// WithForceScrape marks ctx so provider lookups ignore cached negative results:
// the persistent "not found" entries in the JAV lookup cache and the in-memory
// seven-day HTTP 403/404 URL cache. Successful cache hits are still reused, so a
// forced scan only re-checks what previously failed.
//
// It is used by the forced manual directory scan and must be set before the
// lookup starts, because the marker travels with the request context down to the
// HTTP transport.
func WithForceScrape(ctx context.Context) context.Context {
	return context.WithValue(ctx, forceScrapeContextKey{}, true)
}

// IsForceScrape reports whether ctx was marked by WithForceScrape.
func IsForceScrape(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	force, _ := ctx.Value(forceScrapeContextKey{}).(bool)
	return force
}
