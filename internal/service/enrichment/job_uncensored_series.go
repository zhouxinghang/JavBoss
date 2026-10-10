package enrichment

import (
	"context"
	"errors"
	"strings"
	"time"

	"javboss/internal/common"
	"javboss/internal/common/logging"
	"javboss/internal/db"
	"javboss/internal/jav"
)

// StartUncensoredSeriesEnrichment periodically enriches uncensored JAV series.
func StartUncensoredSeriesEnrichment(ctx context.Context, interval time.Duration) {
	startPeriodicJob(ctx, interval, "uncensored jav series", EnrichUncensoredSeries)
}

// EnrichUncensoredSeries fills missing series through JavDB API, then AVSOX.
// Only JAVs explicitly marked uncensored are eligible.
func EnrichUncensoredSeries(ctx context.Context) error {
	if common.DB == nil {
		return errors.New("nil db")
	}
	items, err := db.ListJavsMissingSeries(ctx)
	if err != nil {
		return err
	}
	shuffleCandidates(items)
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if (item.IsUncensored != nil && *item.IsUncensored) != true {
			continue
		}
		code := strings.TrimSpace(item.Code)
		if code == "" {
			continue
		}
		if !enrichmentBackoffAllow(enrichJobUncensoredSeries, item.ID) {
			continue
		}
		// Definite misses are cached by the lookup layer, so only hard provider
		// errors justify delaying the next attempt for this item.
		var hardError bool
		resolved := false
		for _, provider := range []jav.Provider{jav.ProviderJavDBAPI, jav.ProviderAvsox} {
			info, err := jav.LookupJavByCode(ctx, code, provider)
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if err != nil {
				if !errors.Is(err, jav.ErrNotFound) {
					logging.Error("lookup jav series failed provider=%s id=%d code=%s err=%v", provider, item.ID, code, err)
					hardError = true
				}
				continue
			}
			if info == nil {
				continue
			}
			series := strings.TrimSpace(info.Series)
			if series == "" {
				continue
			}
			if updated, err := db.UpdateJavSeriesIfMissing(ctx, item.ID, series); err != nil {
				logging.Error("update jav series failed provider=%s id=%d code=%s err=%v", provider, item.ID, code, err)
				continue
			} else if updated {
				logging.Info("jav series updated provider=%s id=%d code=%s", provider, item.ID, code)
			}
			// A valid result either filled the field or an existing value was preserved.
			resolved = true
			break
		}
		if resolved {
			enrichmentBackoffSucceed(enrichJobUncensoredSeries, item.ID)
		} else if hardError {
			enrichmentBackoffFail(enrichJobUncensoredSeries, item.ID)
		}
	}
	return nil
}
