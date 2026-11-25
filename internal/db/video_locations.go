package db

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"javboss/internal/common"
	"javboss/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrVideoLocationPathConflict is returned when an active location already uses the target path.
var ErrVideoLocationPathConflict = errors.New("video location path already exists")

// AllVideoLocations returns every known video location; used for scan bookkeeping.
func AllVideoLocations(ctx context.Context) ([]models.VideoLocation, error) {
	var locations []models.VideoLocation
	if err := common.DB.WithContext(ctx).Find(&locations).Error; err != nil {
		return nil, fmt.Errorf("load video locations: %w", err)
	}
	return locations, nil
}

// VideoLocationsByDirectory returns every known location in a directory, including hidden rows.
func VideoLocationsByDirectory(ctx context.Context, directoryID int64) ([]models.VideoLocation, error) {
	if directoryID <= 0 {
		return nil, errors.New("directory id cannot be zero")
	}
	var locations []models.VideoLocation
	// Join the belongs-to relation so loading a large directory does not expand
	// every video ID into a Preload IN query and exceed SQLite's parameter limit.
	if err := common.DB.WithContext(ctx).
		Where("video_location.directory_id = ?", directoryID).
		Joins("Video").
		Find(&locations).Error; err != nil {
		return nil, fmt.Errorf("load video locations for directory %d: %w", directoryID, err)
	}
	return locations, nil
}

// UpsertVideoLocation records or updates the on-disk location for a video.
func UpsertVideoLocation(ctx context.Context, videoID, directoryID int64, relativePath string, modifiedAt time.Time) (*models.VideoLocation, error) {
	relativePath = cleanRelativePathForDB(relativePath)
	filename := filepath.Base(filepath.FromSlash(relativePath))
	if videoID <= 0 || directoryID <= 0 || relativePath == "" {
		return nil, errors.New("video_id, directory_id, and relative_path are required")
	}

	loc := models.VideoLocation{
		VideoID:      videoID,
		DirectoryID:  directoryID,
		RelativePath: relativePath,
		Filename:     filename,
		ModifiedAt:   modifiedAt,
		IsDelete:     false,
	}
	tx := common.DB.WithContext(ctx)
	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "directory_id"}, {Name: "relative_path"}},
		DoUpdates: clause.Assignments(map[string]any{
			"video_id":    videoID,
			"filename":    filename,
			"modified_at": modifiedAt,
			"is_delete":   false,
			"updated_at":  gorm.Expr("CURRENT_TIMESTAMP"),
		}),
	}).Create(&loc).Error; err != nil {
		return nil, fmt.Errorf("upsert video location: %w", err)
	}

	var saved models.VideoLocation
	if err := tx.
		Where("directory_id = ? AND relative_path = ?", directoryID, relativePath).
		First(&saved).Error; err != nil {
		return nil, fmt.Errorf("load saved video location: %w", err)
	}
	return &saved, nil
}

// HideVideoLocationsByIDs marks file locations as deleted without deleting video metadata.
func HideVideoLocationsByIDs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	// Leave room for update parameters even with SQLite's older 999-variable
	// limit. Keep all batches atomic, as the original single UPDATE was.
	const batchSize = 400
	err := common.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(ids); start += batchSize {
			batch := ids[start:min(start+batchSize, len(ids))]
			if err := tx.Model(&models.VideoLocation{}).
				Where("id IN ?", batch).
				Update("is_delete", true).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("hide video locations: %w", err)
	}
	return nil
}

// GetVideoIDByPath returns the video ID for an active directory path + relative path.
func GetVideoIDByPath(ctx context.Context, dirPath, relPath string) (int64, error) {
	if strings.TrimSpace(dirPath) == "" || strings.TrimSpace(relPath) == "" {
		return 0, errors.New("directory path and relative path are required")
	}

	var loc models.VideoLocation
	err := common.DB.WithContext(ctx).
		Model(&models.VideoLocation{}).
		Select("video_location.video_id").
		Joins("JOIN directory ON directory.id = video_location.directory_id").
		Joins("JOIN video ON video.id = video_location.video_id").
		Where("directory.path = ?", dirPath).
		Where("video_location.relative_path = ?", cleanRelativePathForDB(relPath)).
		Where(activeLocationWhereSQL("video_location", "directory")).
		Where("COALESCE(directory.missing, 0) = 0").
		First(&loc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("lookup video by path: %w", err)
	}

	return loc.VideoID, nil
}

// GetPrimaryVideoLocation returns the preferred active location for a video.
func GetPrimaryVideoLocation(ctx context.Context, videoID int64) (*models.VideoLocation, error) {
	if videoID <= 0 {
		return nil, errors.New("video id cannot be zero")
	}
	var loc models.VideoLocation
	err := common.DB.WithContext(ctx).
		Model(&models.VideoLocation{}).
		Joins("JOIN directory ON directory.id = video_location.directory_id").
		Where("video_location.video_id = ?", videoID).
		Where(activeLocationWhereSQL("video_location", "directory")).
		Order("video_location.id").
		Preload("DirectoryRef").
		First(&loc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get primary video location: %w", err)
	}
	return &loc, nil
}

// GetActiveVideoLocation returns a specific active location for a video.
func GetActiveVideoLocation(ctx context.Context, videoID, locationID int64) (*models.VideoLocation, error) {
	if videoID <= 0 || locationID <= 0 {
		return nil, errors.New("video id and location id are required")
	}
	var loc models.VideoLocation
	err := common.DB.WithContext(ctx).
		Model(&models.VideoLocation{}).
		Joins("JOIN directory ON directory.id = video_location.directory_id").
		Where("video_location.id = ?", locationID).
		Where("video_location.video_id = ?", videoID).
		Where(activeLocationWhereSQL("video_location", "directory")).
		Preload("DirectoryRef").
		First(&loc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get active video location: %w", err)
	}
	return &loc, nil
}

// VideoLocationPathExists reports whether any active location row already uses the directory/path pair.
func VideoLocationPathExists(ctx context.Context, directoryID int64, relativePath string) (bool, error) {
	relativePath = cleanRelativePathForDB(relativePath)
	if directoryID <= 0 || relativePath == "" {
		return false, errors.New("directory id and relative path are required")
	}
	var count int64
	if err := common.DB.WithContext(ctx).
		Model(&models.VideoLocation{}).
		Where("directory_id = ? AND relative_path = ?", directoryID, relativePath).
		Where("COALESCE(is_delete, 0) = 0").
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check video location path: %w", err)
	}
	return count > 0, nil
}

// UpdateVideoLocationPath updates the stored path metadata for a location after a filesystem rename.
func UpdateVideoLocationPath(ctx context.Context, locationID int64, relativePath string, modifiedAt time.Time) (*models.VideoLocation, error) {
	relativePath = cleanRelativePathForDB(relativePath)
	if locationID <= 0 || relativePath == "" {
		return nil, errors.New("location id and relative path are required")
	}
	updates := map[string]any{
		"relative_path": relativePath,
		"filename":      filepath.Base(filepath.FromSlash(relativePath)),
		"modified_at":   modifiedAt,
		"updated_at":    gorm.Expr("CURRENT_TIMESTAMP"),
	}

	var loc models.VideoLocation
	err := common.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.VideoLocation
		if err := tx.
			Model(&models.VideoLocation{}).
			Where("id = ?", locationID).
			First(&current).Error; err != nil {
			return fmt.Errorf("load current video location: %w", err)
		}

		var activeConflicts int64
		if err := tx.
			Model(&models.VideoLocation{}).
			Where("directory_id = ? AND relative_path = ? AND id <> ?", current.DirectoryID, relativePath, locationID).
			Where("COALESCE(is_delete, 0) = 0").
			Count(&activeConflicts).Error; err != nil {
			return fmt.Errorf("check active video location path: %w", err)
		}
		if activeConflicts > 0 {
			return ErrVideoLocationPathConflict
		}

		if err := tx.
			Where("directory_id = ? AND relative_path = ? AND id <> ?", current.DirectoryID, relativePath, locationID).
			Where("COALESCE(is_delete, 0) <> 0").
			Delete(&models.VideoLocation{}).Error; err != nil {
			return fmt.Errorf("clear hidden video location path: %w", err)
		}

		if err := tx.
			Model(&models.VideoLocation{}).
			Where("id = ?", locationID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("save video location path: %w", err)
		}

		if err := tx.
			Model(&models.VideoLocation{}).
			Where("id = ?", locationID).
			Preload("DirectoryRef").
			Preload("Video").
			Preload("Video.Tags").
			First(&loc).Error; err != nil {
			return fmt.Errorf("load updated video location: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("update video location path: %w", err)
	}
	return &loc, nil
}

func activeVideoLocationSubquery(ctx context.Context) *gorm.DB {
	return common.DB.WithContext(ctx).
		Table("video_location vl").
		Select("1").
		Joins("JOIN directory d ON d.id = vl.directory_id").
		Where("vl.video_id = video.id").
		Where("COALESCE(vl.is_delete, 0) = 0").
		Where("COALESCE(d.is_delete, 0) = 0").
		Where("COALESCE(d.enabled, 1) <> 0")
}

func activeLocationWhereSQL(locationAlias, directoryAlias string) string {
	locationAlias = strings.TrimSpace(locationAlias)
	if locationAlias == "" {
		locationAlias = "video_location"
	}
	directoryAlias = strings.TrimSpace(directoryAlias)
	if directoryAlias == "" {
		directoryAlias = "directory"
	}
	return fmt.Sprintf(
		"COALESCE(%s.is_delete, 0) = 0 AND COALESCE(%s.is_delete, 0) = 0 AND COALESCE(%s.enabled, 1) <> 0",
		locationAlias,
		directoryAlias,
		directoryAlias,
	)
}

func applyDirectoryFilter(q *gorm.DB, locationAlias string, directoryIDs []int64) *gorm.DB {
	if hasZeroID(directoryIDs) {
		return q.Where("1 = 0")
	}
	cleanIDs := uniqueInt64s(directoryIDs)
	if len(cleanIDs) == 0 {
		return q
	}
	locationAlias = strings.TrimSpace(locationAlias)
	if locationAlias == "" {
		locationAlias = "video_location"
	}
	return q.Where(locationAlias+".directory_id IN ?", cleanIDs)
}

func directoryFilterSQL(locationAlias string, directoryIDs []int64) string {
	if hasZeroID(directoryIDs) {
		return " AND 1 = 0"
	}
	cleanIDs := uniqueInt64s(directoryIDs)
	if len(cleanIDs) == 0 {
		return ""
	}
	locationAlias = strings.TrimSpace(locationAlias)
	if locationAlias == "" {
		locationAlias = "video_location"
	}
	parts := make([]string, 0, len(cleanIDs))
	for _, id := range cleanIDs {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return " AND " + locationAlias + ".directory_id IN (" + strings.Join(parts, ",") + ")"
}

func hasZeroID(ids []int64) bool {
	for _, id := range ids {
		if id == 0 {
			return true
		}
	}
	return false
}

func preloadActiveLocations(db *gorm.DB) *gorm.DB {
	return preloadActiveLocationsWhere("")(db)
}

func preloadActiveLocationsWhere(extraWhere string) func(*gorm.DB) *gorm.DB {
	extraWhere = strings.TrimSpace(extraWhere)
	return func(db *gorm.DB) *gorm.DB {
		return db.
			Preload("Locations", func(tx *gorm.DB) *gorm.DB {
				tx = tx.
					Joins("JOIN directory ON directory.id = video_location.directory_id").
					Where(activeLocationWhereSQL("video_location", "directory"))
				if extraWhere != "" {
					tx = tx.Where(extraWhere)
				}
				return tx.Order("video_location.id")
			}).
			Preload("Locations.DirectoryRef")
	}
}

// ReconcileAllVideoPaths is kept for older call sites; video path fields are now
// derived from VideoLocation at read time.
func ReconcileAllVideoPaths(ctx context.Context) error {
	_ = ctx
	return nil
}

// DirectoryCleanupVideoMeta holds lightweight metadata for active video locations
// inside one directory, used by the duplicate-file cleanup scanner.
type DirectoryCleanupVideoMeta struct {
	LocationID        int64
	VideoID           int64
	RelativePath      string
	JavID             *int64
	JavCode           string
	JavTitle          string
	JavZhTitle        string
	DurationSec       int64
	Fingerprint       string
	JavScrapeOverride string
}

// ListDirectoryCleanupVideoMetadata returns active video location metadata for one directory,
// keyed by cleaned slash-separated relative path.
func ListDirectoryCleanupVideoMetadata(ctx context.Context, directoryID int64) (map[string]DirectoryCleanupVideoMeta, error) {
	if directoryID <= 0 {
		return nil, errors.New("directory id cannot be zero")
	}
	if common.DB == nil {
		return nil, nil
	}
	var rows []DirectoryCleanupVideoMeta
	if err := common.DB.WithContext(ctx).
		Table("video_location vl").
		Select(`vl.id AS location_id,
			vl.video_id AS video_id,
			vl.relative_path AS relative_path,
			vl.jav_id AS jav_id,
			COALESCE(j.code, '') AS jav_code,
			COALESCE(j.title, '') AS jav_title,
			COALESCE(j.zh_title, '') AS jav_zh_title,
			v.duration_sec AS duration_sec,
			COALESCE(v.fingerprint, '') AS fingerprint,
			COALESCE(v.jav_scrape_override, '') AS jav_scrape_override`).
		Joins("JOIN video v ON v.id = vl.video_id").
		Joins("LEFT JOIN jav j ON j.id = vl.jav_id").
		Where("vl.directory_id = ?", directoryID).
		Where("COALESCE(vl.is_delete, 0) = 0").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load directory cleanup video metadata: %w", err)
	}
	result := make(map[string]DirectoryCleanupVideoMeta, len(rows))
	for i := range rows {
		key := cleanRelativePathForDB(rows[i].RelativePath)
		if key == "" {
			continue
		}
		rows[i].RelativePath = key
		result[key] = rows[i]
	}
	return result, nil
}

// HideDirectoryVideoLocationsByPaths marks matching active locations in one directory as deleted
// and returns the number of rows updated.
func HideDirectoryVideoLocationsByPaths(ctx context.Context, directoryID int64, relativePaths []string) (int64, error) {
	if directoryID <= 0 || len(relativePaths) == 0 || common.DB == nil {
		return 0, nil
	}
	cleaned := make([]string, 0, len(relativePaths))
	seen := make(map[string]struct{}, len(relativePaths))
	for _, raw := range relativePaths {
		p := cleanRelativePathForDB(raw)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		cleaned = append(cleaned, p)
	}
	if len(cleaned) == 0 {
		return 0, nil
	}
	const batchSize = 400
	var hidden int64
	err := common.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(cleaned); start += batchSize {
			batch := cleaned[start:min(start+batchSize, len(cleaned))]
			res := tx.Model(&models.VideoLocation{}).
				Where("directory_id = ?", directoryID).
				Where("relative_path IN ?", batch).
				Where("COALESCE(is_delete, 0) = 0").
				Update("is_delete", true)
			if res.Error != nil {
				return res.Error
			}
			hidden += res.RowsAffected
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("hide directory video locations by paths: %w", err)
	}
	return hidden, nil
}

func cleanRelativePathForDB(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	cleaned := filepath.Clean(filepath.FromSlash(p))
	if cleaned == "." {
		return ""
	}
	return filepath.ToSlash(cleaned)
}
