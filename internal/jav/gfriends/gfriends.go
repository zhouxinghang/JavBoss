// Package gfriends resolves idol avatar image URLs from the Gfriends repository.
//
// The repository publishes a Filetree.json that maps actress names to stored
// image files grouped by shard directory. The lookup mirrors gfriends-inputer:
// the name is matched exactly first and then with a parenthesized suffix such as
// "（…）" or "(…)" removed. Entries prefixed with "AI-Fix-" are treated as an
// optimized copy of the same actress.
package gfriends

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"javboss/internal/common/logging"
	"javboss/internal/util"
)

// DefaultRepositoryURL is the official Gfriends repository raw base URL.
const DefaultRepositoryURL = "https://raw.githubusercontent.com/gfriends/gfriends/master/"

const (
	defaultTreeTTL       = 24 * time.Hour
	maxTreeResponseBytes = 64 << 20 // 64 MiB guard for the file tree payload
)

var (
	fullWidthSuffixPattern = regexp.MustCompile(`（[^）]*）`)
	halfWidthSuffixPattern = regexp.MustCompile(`\([^)]*\)`)
)

// ErrTreeUnavailable indicates that the repository file tree could not be loaded.
var ErrTreeUnavailable = errors.New("gfriends file tree unavailable")

// Image is one avatar candidate for an actress: the source shard (label) it
// came from and the downloadable URL.
type Image struct {
	Source string
	URL    string
}

// Client loads and caches the Gfriends repository file tree.
type Client struct {
	repoURL    string
	httpClient *http.Client
	treePath   string
	ttl        time.Duration
	now        func() time.Time

	mu        sync.RWMutex
	tree      map[string][]Image
	fetchedAt time.Time

	// loadMu serializes cold-cache fetches so concurrent requests do not all
	// download the multi-megabyte file tree at once.
	loadMu sync.Mutex
}

// Option customizes a Client.
type Option func(*Client)

// WithHTTPClient sets the HTTP client used for repository requests.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithRepositoryURL overrides the repository base URL. A trailing slash is added.
func WithRepositoryURL(repoURL string) Option {
	return func(c *Client) {
		repoURL = strings.TrimSpace(repoURL)
		if repoURL != "" {
			if !strings.HasSuffix(repoURL, "/") {
				repoURL += "/"
			}
			c.repoURL = repoURL
		}
	}
}

// WithCachePath stores the downloaded file tree on disk for offline reuse.
func WithCachePath(path string) Option {
	return func(c *Client) {
		c.treePath = strings.TrimSpace(path)
	}
}

// WithTTL sets how long a cached file tree stays fresh.
func WithTTL(ttl time.Duration) Option {
	return func(c *Client) {
		if ttl > 0 {
			c.ttl = ttl
		}
	}
}

// WithClock overrides the clock, primarily for tests.
func WithClock(now func() time.Time) Option {
	return func(c *Client) {
		if now != nil {
			c.now = now
		}
	}
}

// New creates a Gfriends client.
func New(opts ...Option) *Client {
	c := &Client{
		repoURL:    DefaultRepositoryURL,
		httpClient: util.DefaultHTTPClient(),
		ttl:        defaultTreeTTL,
		now:        time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

type fileTree struct {
	Information struct {
		TotalNum int `json:"TotalNum"`
	} `json:"Information"`
	Content map[string]map[string]string `json:"Content"`
}

// Lookup returns candidate images for the first matching name.
//
// Names are tried in order, and each name is matched exactly first and then
// with a full-width or half-width parenthesized suffix removed.
func (c *Client) Lookup(ctx context.Context, names ...string) []Image {
	if c == nil {
		return nil
	}
	tree, err := c.loadTree(ctx)
	if err != nil {
		logging.Error("gfriends: load file tree: %v", err)
		return nil
	}
	for _, name := range names {
		for _, candidate := range nameCandidates(name) {
			if images := tree[candidate]; len(images) > 0 {
				return images
			}
		}
	}
	return nil
}

// TreeSize returns the number of actresses known to the loaded file tree.
func (c *Client) TreeSize(ctx context.Context) int {
	if c == nil {
		return 0
	}
	tree, err := c.loadTree(ctx)
	if err != nil {
		return 0
	}
	return len(tree)
}

func (c *Client) loadTree(ctx context.Context) (map[string][]Image, error) {
	if tree, ok := c.cachedTree(); ok {
		return tree, nil
	}
	c.loadMu.Lock()
	defer c.loadMu.Unlock()
	if tree, ok := c.cachedTree(); ok {
		return tree, nil
	}
	tree, raw, err := c.fetchTree(ctx)
	if err != nil {
		if fallback, cacheErr := c.readCacheFile(); cacheErr == nil {
			c.storeTree(fallback)
			logging.Info("gfriends: using stale file tree cache after fetch failure: %v", err)
			return fallback, nil
		}
		return nil, err
	}
	c.storeTree(tree)
	c.writeCacheFile(raw)
	return tree, nil
}

func (c *Client) cachedTree() (map[string][]Image, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.tree == nil {
		return nil, false
	}
	if c.ttl > 0 && c.now().Sub(c.fetchedAt) >= c.ttl {
		return nil, false
	}
	return c.tree, true
}

func (c *Client) storeTree(tree map[string][]Image) {
	c.mu.Lock()
	c.tree = tree
	c.fetchedAt = c.now()
	c.mu.Unlock()
}

func (c *Client) fetchTree(ctx context.Context) (map[string][]Image, []byte, error) {
	if c.repoURL == "" {
		return nil, nil, fmt.Errorf("%w: repository url is empty", ErrTreeUnavailable)
	}
	treeURL := c.repoURL + "Filetree.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, treeURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build gfriends request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "JavBoss")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrTreeUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("%w: status %s", ErrTreeUnavailable, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTreeResponseBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: read body: %v", ErrTreeUnavailable, err)
	}
	tree, err := parseFileTree(data, c.repoURL)
	if err != nil {
		return nil, nil, err
	}
	return tree, data, nil
}

func parseFileTree(data []byte, repoURL string) (map[string][]Image, error) {
	var raw fileTree
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: decode file tree: %v", ErrTreeUnavailable, err)
	}
	base := strings.TrimRight(repoURL, "/") + "/Content/"
	out := make(map[string][]Image)
	for shard, files := range raw.Content {
		shard = strings.TrimSpace(shard)
		if shard == "" {
			continue
		}
		for displayName, storedName := range files {
			name := normalizeEntryName(displayName)
			storedName = strings.TrimSpace(storedName)
			if name == "" || storedName == "" {
				continue
			}
			imageURL := base + url.PathEscape(shard) + "/" + escapeStoredName(storedName)
			out[name] = append(out[name], Image{Source: shard, URL: imageURL})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: file tree contains no entries", ErrTreeUnavailable)
	}
	// Deterministic candidate order so repeated lookups pick the same avatar.
	// Prefer AI-optimized copies, which Gfriends Inputer also defaults to.
	for _, images := range out {
		slices.SortStableFunc(images, func(a, b Image) int {
			aFix := strings.Contains(a.URL, "AI-Fix-")
			bFix := strings.Contains(b.URL, "AI-Fix-")
			if aFix != bFix {
				if aFix {
					return -1
				}
				return 1
			}
			if diff := strings.Compare(a.Source, b.Source); diff != 0 {
				return diff
			}
			return strings.Compare(a.URL, b.URL)
		})
	}
	return out, nil
}

// escapeStoredName escapes a stored file name while preserving an optional
// cache-busting query token such as "name.jpg?t=1658331538" that Gfriends uses.
func escapeStoredName(storedName string) string {
	pathPart := storedName
	queryPart := ""
	if index := strings.Index(storedName, "?"); index >= 0 {
		pathPart = storedName[:index]
		queryPart = storedName[index:]
	}
	return url.PathEscape(pathPart) + queryPart
}

func normalizeEntryName(displayName string) string {
	name := strings.TrimSpace(displayName)
	if ext := filepath.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	return strings.TrimSpace(strings.TrimPrefix(name, "AI-Fix-"))
}

// nameCandidates returns the exact name plus parenthesized-suffix stripped variants.
func nameCandidates(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	candidates := []string{name}
	for _, stripped := range []string{
		fullWidthSuffixPattern.ReplaceAllString(name, ""),
		halfWidthSuffixPattern.ReplaceAllString(name, ""),
	} {
		stripped = strings.TrimSpace(stripped)
		if stripped == "" || stripped == name {
			continue
		}
		candidates = append(candidates, stripped)
	}
	return candidates
}

func (c *Client) readCacheFile() (map[string][]Image, error) {
	if c.treePath == "" {
		return nil, errors.New("no cache path")
	}
	data, err := os.ReadFile(c.treePath)
	if err != nil {
		return nil, err
	}
	return parseFileTree(data, c.repoURL)
}

func (c *Client) writeCacheFile(raw []byte) {
	if c.treePath == "" || len(raw) == 0 {
		return
	}
	if err := os.MkdirAll(filepath.Dir(c.treePath), 0o755); err != nil {
		logging.Error("gfriends: create cache dir: %v", err)
		return
	}
	tmp := c.treePath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		logging.Error("gfriends: write cache: %v", err)
		return
	}
	if err := os.Rename(tmp, c.treePath); err != nil {
		_ = os.Remove(tmp)
		logging.Error("gfriends: finalize cache: %v", err)
	}
}
