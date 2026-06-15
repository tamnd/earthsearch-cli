// Package earthsearch is the library behind the earthsearch command line:
// the HTTP client, wire types, and typed data models for the AWS Earth Search
// STAC API (SpatioTemporal Asset Catalog).
package earthsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const Host = "earth-search.aws.element84.com"
const BaseURL = "https://earth-search.aws.element84.com/v1"

// Config holds tunable knobs for a Client.
type Config struct {
	BaseURL   string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   BaseURL,
		Rate:      500 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
		UserAgent: "earthsearch-cli/0.1.0 (github.com/tamnd/earthsearch-cli)",
	}
}

// Client talks to the Earth Search STAC API.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client with default config.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// NewClientAt returns a Client targeting a custom base URL (useful in tests).
func NewClientAt(baseURL string) *Client {
	c := NewClient()
	c.cfg.BaseURL = baseURL
	return c
}

// --- wire types (unexported) ---

type wireItem struct {
	ID         string               `json:"id"`
	Type       string               `json:"type"`
	Collection string               `json:"collection"`
	BBox       []float64            `json:"bbox"`
	Properties wireProperties       `json:"properties"`
	Assets     map[string]wireAsset `json:"assets"`
}

type wireProperties struct {
	DateTime      string   `json:"datetime"`
	Platform      string   `json:"platform"`
	Constellation string   `json:"constellation"`
	CloudCover    float64  `json:"eo:cloud_cover"`
	Instruments   []string `json:"instruments"`
}

type wireAsset struct {
	Href  string `json:"href"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

type wireSearchResp struct {
	Type     string      `json:"type"`
	Context  wireContext `json:"context"`
	Features []wireItem  `json:"features"`
	Links    []wireLink  `json:"links"`
}

type wireContext struct {
	Limit    int `json:"limit"`
	Matched  int `json:"matched"`
	Returned int `json:"returned"`
}

type wireLink struct {
	Rel  string        `json:"rel"`
	Body *wireLinkBody `json:"body,omitempty"`
}

type wireLinkBody struct {
	Next        string   `json:"next"`
	Collections []string `json:"collections"`
	Limit       int      `json:"limit"`
}

type wireCollection struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type wireCollectionsResp struct {
	Collections []wireCollection `json:"collections"`
}

// --- public types ---

// Item is a single STAC item (satellite imagery scene).
type Item struct {
	ID            string  `json:"id"                kit:"id"`
	Collection    string  `json:"collection,omitempty"`
	DateTime      string  `json:"datetime,omitempty"`
	Platform      string  `json:"platform,omitempty"`
	Constellation string  `json:"constellation,omitempty"`
	CloudCover    float64 `json:"cloud_cover,omitempty"`
	BBoxMinLon    float64 `json:"bbox_min_lon,omitempty"`
	BBoxMinLat    float64 `json:"bbox_min_lat,omitempty"`
	BBoxMaxLon    float64 `json:"bbox_max_lon,omitempty"`
	BBoxMaxLat    float64 `json:"bbox_max_lat,omitempty"`
	Thumbnail     string  `json:"thumbnail,omitempty"`
}

// Collection is a named group of satellite imagery scenes.
type Collection struct {
	ID          string `json:"id"           kit:"id"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// --- client methods ---

// SearchItems posts a STAC search request and returns (items, totalMatched, nextToken, error).
func (c *Client) SearchItems(ctx context.Context, collections []string, limit int, nextToken string) ([]*Item, int, string, error) {
	return c.SearchItemsAt(ctx, c.cfg.BaseURL, collections, limit, nextToken)
}

// SearchItemsAt is like SearchItems but targets a custom base URL (useful in tests).
func (c *Client) SearchItemsAt(ctx context.Context, baseURL string, collections []string, limit int, nextToken string) ([]*Item, int, string, error) {
	if limit <= 0 {
		limit = 10
	}

	// Build the request body; omit "next" when empty.
	type searchBody struct {
		Limit       int      `json:"limit"`
		Collections []string `json:"collections,omitempty"`
		Next        string   `json:"next,omitempty"`
	}
	payload := searchBody{
		Limit:       limit,
		Collections: collections,
		Next:        nextToken,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, "", fmt.Errorf("marshal search body: %w", err)
	}

	body, err := c.post(ctx, baseURL+"/search", raw)
	if err != nil {
		return nil, 0, "", err
	}

	var resp wireSearchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, "", fmt.Errorf("decode search response: %w", err)
	}

	items := make([]*Item, 0, len(resp.Features))
	for _, f := range resp.Features {
		items = append(items, itemFromWire(f))
	}

	var next string
	for _, l := range resp.Links {
		if l.Rel == "next" && l.Body != nil {
			next = l.Body.Next
			break
		}
	}

	return items, resp.Context.Matched, next, nil
}

// ListCollections returns all available satellite imagery collections.
func (c *Client) ListCollections(ctx context.Context) ([]*Collection, error) {
	return c.ListCollectionsAt(ctx, c.cfg.BaseURL)
}

// ListCollectionsAt is like ListCollections but targets a custom base URL (useful in tests).
func (c *Client) ListCollectionsAt(ctx context.Context, baseURL string) ([]*Collection, error) {
	body, err := c.get(ctx, baseURL+"/collections")
	if err != nil {
		return nil, err
	}

	var resp wireCollectionsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode collections response: %w", err)
	}

	out := make([]*Collection, 0, len(resp.Collections))
	for _, wc := range resp.Collections {
		out = append(out, &Collection{
			ID:          wc.ID,
			Title:       wc.Title,
			Description: wc.Description,
		})
	}
	return out, nil
}

// --- internal HTTP helpers ---

func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.doGet(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) post(ctx context.Context, url string, payload []byte) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.doPost(ctx, url, payload)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("post %s: %w", url, lastErr)
}

func (c *Client) doGet(ctx context.Context, url string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	return c.execute(req)
}

func (c *Client) doPost(ctx context.Context, url string, payload []byte) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Content-Type", "application/json")
	return c.execute(req)
}

func (c *Client) execute(req *http.Request) (body []byte, retry bool, err error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// itemFromWire converts a wire STAC item to the public Item type.
func itemFromWire(w wireItem) *Item {
	item := &Item{
		ID:            w.ID,
		Collection:    w.Collection,
		DateTime:      w.Properties.DateTime,
		Platform:      w.Properties.Platform,
		Constellation: w.Properties.Constellation,
		CloudCover:    w.Properties.CloudCover,
	}
	if len(w.BBox) >= 4 {
		item.BBoxMinLon = w.BBox[0]
		item.BBoxMinLat = w.BBox[1]
		item.BBoxMaxLon = w.BBox[2]
		item.BBoxMaxLat = w.BBox[3]
	}
	if a, ok := w.Assets["thumbnail"]; ok {
		item.Thumbnail = a.Href
	}
	return item
}
