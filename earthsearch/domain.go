package earthsearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

func init() { kit.Register(Domain{}) }

// Domain is the earthsearch driver for the AWS Earth Search STAC API.
type Domain struct{}

func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "earthsearch",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "earthsearch",
			Short:  "Search 130M+ satellite imagery scenes via AWS Earth Search STAC API.",
			Long: `Search 130M+ satellite imagery scenes from Sentinel-2, Landsat, and more.

earthsearch reads from the AWS Earth Search STAC (SpatioTemporal Asset Catalog) API,
including imagery from Sentinel-1, Sentinel-2, Landsat, NAIP, and Copernicus DEM.
No API key required.`,
			Site: Host,
			Repo: "https://github.com/tamnd/earthsearch-cli",
		},
	}
}

func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search satellite imagery by collection (--collection, --limit)",
		Args:    []kit.Arg{{Name: "collection", Help: "collection ID e.g. sentinel-2-l2a"}}}, searchItems)

	kit.Handle(app, kit.OpMeta{Name: "collections", Group: "read", List: true,
		Summary: "List available satellite imagery collections"}, listCollections)
}

func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.cfg.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.cfg.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.cfg.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.http.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type searchInput struct {
	Collection string  `kit:"arg"          help:"collection ID e.g. sentinel-2-l2a"`
	Limit      int     `kit:"flag,inherit" help:"max results"`
	Client     *Client `kit:"inject"`
}

type collectionsInput struct {
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchItems(ctx context.Context, in searchInput, emit func(*Item) error) error {
	var collections []string
	if in.Collection != "" {
		collections = []string{in.Collection}
	}
	items, _, _, err := in.Client.SearchItems(ctx, collections, in.Limit, "")
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := emit(item); err != nil {
			return err
		}
	}
	return nil
}

func listCollections(ctx context.Context, in collectionsInput, emit func(*Collection) error) error {
	cols, err := in.Client.ListCollections(ctx)
	if err != nil {
		return err
	}
	for _, col := range cols {
		if err := emit(col); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver ---

func (Domain) Classify(input string) (string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty earthsearch reference")
	}
	return "item", input, nil
}

func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "item":
		return fmt.Sprintf("https://earth-search.aws.element84.com/v1/search?ids=%s", id), nil
	default:
		return "", errs.Usage("earthsearch has no resource type %q", uriType)
	}
}
