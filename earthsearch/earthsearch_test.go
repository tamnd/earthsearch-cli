package earthsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearchItems(t *testing.T) {
	resp := wireSearchResp{
		Type: "FeatureCollection",
		Context: wireContext{
			Limit:    2,
			Matched:  45794654,
			Returned: 2,
		},
		Features: []wireItem{
			{
				ID:         "S2B_37SDA_20260615_0_L2A",
				Type:       "Feature",
				Collection: "sentinel-2-l2a",
				BBox:       []float64{36.1, 35.9, 37.2, 36.9},
				Properties: wireProperties{
					DateTime:      "2026-06-15T08:19:44.884000Z",
					Platform:      "sentinel-2b",
					Constellation: "sentinel-2",
					CloudCover:    12.5,
				},
				Assets: map[string]wireAsset{
					"thumbnail": {Href: "https://example.com/thumb.jpg"},
				},
			},
			{
				ID:         "S2A_37SDA_20260614_0_L2A",
				Type:       "Feature",
				Collection: "sentinel-2-l2a",
				BBox:       []float64{36.0, 35.8, 37.1, 36.8},
				Properties: wireProperties{
					DateTime: "2026-06-14T08:20:00.000000Z",
					Platform: "sentinel-2a",
				},
				Assets: map[string]wireAsset{},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/search" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClientAt(srv.URL)
	c.cfg.Rate = 0

	items, total, next, err := c.SearchItemsAt(context.Background(), srv.URL, []string{"sentinel-2-l2a"}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 45794654 {
		t.Errorf("total = %d, want 45794654", total)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != "S2B_37SDA_20260615_0_L2A" {
		t.Errorf("items[0].ID = %q, want S2B_37SDA_20260615_0_L2A", items[0].ID)
	}
	if items[0].Collection != "sentinel-2-l2a" {
		t.Errorf("items[0].Collection = %q, want sentinel-2-l2a", items[0].Collection)
	}
	if items[0].DateTime != "2026-06-15T08:19:44.884000Z" {
		t.Errorf("items[0].DateTime = %q", items[0].DateTime)
	}
	if items[0].Thumbnail != "https://example.com/thumb.jpg" {
		t.Errorf("items[0].Thumbnail = %q", items[0].Thumbnail)
	}
	if next != "" {
		t.Errorf("next = %q, want empty", next)
	}
}

func TestSearchItemsNextToken(t *testing.T) {
	resp := wireSearchResp{
		Type:    "FeatureCollection",
		Context: wireContext{Limit: 2, Matched: 1000, Returned: 2},
		Features: []wireItem{
			{ID: "item-1", BBox: []float64{0, 0, 1, 1}},
			{ID: "item-2", BBox: []float64{1, 1, 2, 2}},
		},
		Links: []wireLink{
			{
				Rel: "next",
				Body: &wireLinkBody{
					Next:        "token123",
					Collections: []string{"sentinel-2-l2a"},
					Limit:       2,
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClientAt(srv.URL)
	c.cfg.Rate = 0

	_, _, nextToken, err := c.SearchItemsAt(context.Background(), srv.URL, []string{"sentinel-2-l2a"}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if nextToken != "token123" {
		t.Errorf("nextToken = %q, want token123", nextToken)
	}
}

func TestListCollections(t *testing.T) {
	resp := wireCollectionsResp{
		Collections: []wireCollection{
			{ID: "sentinel-2-l2a", Title: "Sentinel-2 L2A", Description: "Sentinel-2 Level-2A"},
			{ID: "landsat-c2-l2", Title: "Landsat Collection 2 Level-2", Description: "Landsat C2 L2"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/collections" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClientAt(srv.URL)
	c.cfg.Rate = 0

	cols, err := c.ListCollectionsAt(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 2 {
		t.Fatalf("len(cols) = %d, want 2", len(cols))
	}
	if cols[0].ID != "sentinel-2-l2a" {
		t.Errorf("cols[0].ID = %q, want sentinel-2-l2a", cols[0].ID)
	}
	if cols[0].Title != "Sentinel-2 L2A" {
		t.Errorf("cols[0].Title = %q, want Sentinel-2 L2A", cols[0].Title)
	}
}

func TestRetryOn503(t *testing.T) {
	var hits int
	resp := wireSearchResp{
		Type:    "FeatureCollection",
		Context: wireContext{Limit: 1, Matched: 1, Returned: 1},
		Features: []wireItem{
			{ID: "item-ok", BBox: []float64{0, 0, 1, 1}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClientAt(srv.URL)
	c.cfg.Rate = 0
	c.cfg.Retries = 5

	start := time.Now()
	items, _, _, err := c.SearchItemsAt(context.Background(), srv.URL, nil, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "item-ok" {
		t.Errorf("unexpected items after retry: %v", items)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestBackoff(t *testing.T) {
	if got := backoff(1); got != 500*time.Millisecond {
		t.Errorf("backoff(1) = %v, want 500ms", got)
	}
	if got := backoff(10); got != 5*time.Second {
		t.Errorf("backoff(10) = %v, want 5s", got)
	}
}
