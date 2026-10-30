package db

import (
	"context"
	"fmt"
	"strings"

	"javboss/internal/common"
	"javboss/internal/models"
)

// ListJavsMissingZhTitle bounds each translation pass and excludes titles which
// cannot be translated. Random ordering lets later items progress after failures.
func ListJavsMissingZhTitle(ctx context.Context, limit int) ([]models.Jav, error) {
	var items []models.Jav
	err := common.DB.WithContext(ctx).Select("id", "title").
		Where("TRIM(COALESCE(zh_title, '')) = '' AND TRIM(COALESCE(title, '')) <> ''").
		Order("RANDOM()").Limit(limit).Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list javs missing Chinese titles: %w", err)
	}
	return items, nil
}

// SetJavZhTitleIfMissing preserves titles added during translation and discards
// a translation when the original title has changed since it was requested.
func SetJavZhTitleIfMissing(ctx context.Context, id int64, originalTitle, zhTitle string) (bool, error) {
	zhTitle = strings.TrimSpace(zhTitle)
	if zhTitle == "" {
		return false, nil
	}
	result := common.DB.WithContext(ctx).Model(&models.Jav{}).
		Where("id = ? AND title = ? AND TRIM(COALESCE(zh_title, '')) = ''", id, originalTitle).
		Update("zh_title", zhTitle)
	if result.Error != nil {
		return false, fmt.Errorf("save jav Chinese title: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}
