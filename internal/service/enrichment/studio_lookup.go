package enrichment

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	"javboss/internal/common/logging"
	"javboss/internal/db"
	"javboss/internal/jav"
)

// enrichJavStudio reconciles studio names for one JAV. It reports whether any
// provider failed with a retryable (non-ErrNotFound) error so the caller can
// back off before hammering the providers again.
func enrichJavStudio(ctx context.Context, item db.JavEnrichmentItem, providers []jav.Provider, lookup func(context.Context, string, jav.Provider) (*jav.JavInfo, error)) (bool, error) {
	code := strings.TrimSpace(item.Code)
	if code == "" {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// Each provider writes its own slot so name priority is independent of
	// response order. Only one movie's providers are in flight at a time.
	names := make([]string, len(providers))
	var hardError atomic.Bool
	var pending sync.WaitGroup
	for i, provider := range providers {
		pending.Add(1)
		go func() {
			defer pending.Done()
			info, err := lookup(ctx, code, provider)
			if err != nil {
				if ctx.Err() == nil && !errors.Is(err, jav.ErrNotFound) {
					logging.Error("lookup jav studio failed provider=%s id=%d code=%s err=%v", provider, item.ID, code, err)
					hardError.Store(true)
				}
				return
			}
			if info != nil {
				names[i] = strings.TrimSpace(info.Studio)
			}
		}()
	}
	pending.Wait()
	if err := ctx.Err(); err != nil {
		return hardError.Load(), err
	}
	if updated, err := db.ReconcileJavStudioNames(ctx, item.ID, item.StudioID, names); err != nil {
		return hardError.Load(), err
	} else if updated {
		logging.Info("jav studio names reconciled id=%d code=%s names=%q", item.ID, code, names)
	}
	return hardError.Load(), nil
}
