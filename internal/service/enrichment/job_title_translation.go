package enrichment

import (
	"context"
	"fmt"
	"time"

	"javboss/internal/common/logging"
	"javboss/internal/db"
	"javboss/internal/deepseek"
	"javboss/internal/util"
)

var titleTranslationClient = util.NewHTTPClient(45 * time.Second)

type titleTranslator func(context.Context, deepseek.Settings, string) (string, error)

func TranslateTitle(ctx context.Context, settings deepseek.Settings, title string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return deepseek.TranslateTitle(ctx, titleTranslationClient, settings, title)
}

func StartTitleTranslationEnrichment(ctx context.Context, interval time.Duration) {
	startPeriodicJob(ctx, interval, "jav Chinese title", func(ctx context.Context) error {
		return enrichTitleTranslations(ctx, TranslateTitle)
	})
}

func enrichTitleTranslations(ctx context.Context, translate titleTranslator) error {
	config, err := db.ListConfig(ctx)
	if err != nil {
		return err
	}
	settings := deepseek.SettingsFromConfig(config)
	if !settings.AutoTranslate || settings.APIKey == "" {
		return nil
	}
	items, err := db.ListJavsMissingZhTitle(ctx, 20)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Re-read settings so disabling translation stops an active batch.
		config, err := db.ListConfig(ctx)
		if err != nil {
			return err
		}
		settings = deepseek.SettingsFromConfig(config)
		if !settings.AutoTranslate || settings.APIKey == "" {
			return nil
		}
		zhTitle, err := translate(ctx, settings, item.Title)
		if err != nil {
			// End this pass on errors (including authentication/rate limiting) to
			// avoid repeatedly sending requests with a broken configuration.
			return fmt.Errorf("translate jav title id=%d: %w", item.ID, err)
		}
		if updated, err := db.SetJavZhTitleIfMissing(ctx, item.ID, item.Title, zhTitle); err != nil {
			return err
		} else if updated {
			logging.Info("jav Chinese title updated id=%d", item.ID)
		}
	}
	return nil
}
