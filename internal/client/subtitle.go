package client

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"javboss/internal/subtitle"
)

// Download an authenticated snapshot before playback. MPV receives only a local
// file path, so it never needs the server session cookie to load subtitles.
func (c *Client) prepareSubtitle(ctx context.Context, videoID int64, cookie string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	remote := c.remoteState()
	if remote == nil {
		return "", fmt.Errorf("subtitle server is not configured")
	}
	endpoint := "/videos/" + strconv.FormatInt(videoID, 10) + "/subtitle"
	headers := make(http.Header)
	headers.Set("Cookie", cookie)
	response, err := c.remoteRequest(ctx, http.MethodGet, endpoint, "", nil, headers)
	if err != nil {
		return "", fmt.Errorf("get remote subtitle: %w", err)
	}
	defer response.Body.Close()
	// An older server has no subtitle endpoint and remains usable.
	if response.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get remote subtitle: HTTP %d", response.StatusCode)
	}
	var metadata struct {
		Subtitle *subtitle.File `json:"subtitle"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&metadata); err != nil {
		return "", fmt.Errorf("decode remote subtitle: %w", err)
	}
	if metadata.Subtitle == nil {
		return "", nil
	}
	fileResponse, err := c.remoteRequest(ctx, http.MethodGet, endpoint+"/file", "", nil, headers)
	if err != nil {
		return "", fmt.Errorf("download remote subtitle: %w", err)
	}
	defer fileResponse.Body.Close()
	if fileResponse.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download remote subtitle: HTTP %d", fileResponse.StatusCode)
	}
	// Video IDs from different servers must never share a cached selection.
	cacheDir := filepath.Join(c.clientDataDir(), "remote-subtitles", fmt.Sprintf("%x", sha256.Sum256([]byte(remote.base.String()))))
	file, err := subtitle.Save(cacheDir, videoID, metadata.Subtitle.Name, fileResponse.Body)
	if err != nil {
		return "", fmt.Errorf("cache remote subtitle: %w", err)
	}
	if c.remoteState() != remote {
		return "", fmt.Errorf("subtitle server changed during playback preparation")
	}
	return file.Path, nil
}
