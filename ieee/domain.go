package ieee

import (
	"context"
	"fmt"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// init registers the IEEE domain so ant can load it with a blank import.
func init() { kit.Register(Domain{}) }

// Domain is the IEEE driver. No state; the per-run client is built by the factory.
type Domain struct{}

// Info describes the scheme and identity for this domain.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "ieee",
		Hosts:  []string{IEEEHost, CrossRefHost},
		Identity: kit.Identity{
			Binary: "ieee",
			Short:  "Search IEEE Xplore papers from the command line",
			Long: `Search IEEE Xplore papers from the command line.

ieee fetches paper metadata from the CrossRef public API (api.crossref.org),
filtered to IEEE publications (DOI prefix 10.1109). CrossRef is the
policy-compliant, open alternative to ieeexplore.ieee.org, which blocks
non-browser clients via CloudFront WAF.

No API key is required. Full text requires an institutional subscription;
this CLI retrieves metadata only.

Quick start:
  ieee search "deep learning" -n 10
  ieee search "transformer" --year-start 2017 --type conference
  ieee paper 10.1109/cvpr.2016.90
  ieee top --subject machine-learning -n 20
  ieee search "neural network" -o json | jq '.[].doi'`,
			Site: IEEEHost,
			Repo: "https://github.com/tamnd/ieee-cli",
		},
	}
}

// Register installs the client factory and all operations onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{
		Name:    "search",
		Group:   "papers",
		Summary: "Search IEEE papers via CrossRef",
		Args:    []kit.Arg{{Name: "query", Help: "search terms (e.g. \"deep learning\")"}},
	}, searchPapers)

	kit.Handle(app, kit.OpMeta{
		Name:     "paper",
		Group:    "papers",
		Summary:  "Fetch a single paper by DOI or IEEE Xplore URL",
		Single:   true,
		URIType:  "paper",
		Resolver: true,
		Args:     []kit.Arg{{Name: "ref", Help: "DOI (e.g. 10.1109/cvpr.2016.90) or Xplore URL"}},
	}, getPaper)

	kit.Handle(app, kit.OpMeta{
		Name:    "top",
		Group:   "papers",
		Summary: "List top IEEE papers by citation count",
	}, topPapers)
}

// newClient builds a Client from the resolved kit Config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := DefaultConfig()
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.Timeout = cfg.Timeout
	}
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	return NewClient(c), nil
}

// --- input structs ---

type searchInput struct {
	Query     string  `kit:"arg" help:"search terms"`
	YearStart int     `kit:"flag" help:"earliest publication year" default:"0"`
	YearEnd   int     `kit:"flag" help:"latest publication year" default:"0"`
	DocType   string  `kit:"flag" help:"document type: journal, conference, standard" default:""`
	Limit     int     `kit:"flag,inherit" help:"max results" default:"20"`
	Client    *Client `kit:"inject"`
}

type paperInput struct {
	Ref    string  `kit:"arg" help:"DOI or IEEE Xplore URL"`
	Client *Client `kit:"inject"`
}

type topInput struct {
	Subject string  `kit:"flag" help:"subject area (e.g. machine-learning, computer-vision)" default:""`
	Limit   int     `kit:"flag,inherit" help:"max results" default:"20"`
	Client  *Client `kit:"inject"`
}

// --- handlers ---

func searchPapers(ctx context.Context, in searchInput, emit func(Paper) error) error {
	if in.Query == "" {
		return errs.Usage("query is required (e.g. ieee search \"deep learning\")")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	papers, err := in.Client.SearchPapers(ctx, in.Query, SearchOptions{
		YearStart: in.YearStart,
		YearEnd:   in.YearEnd,
		DocType:   in.DocType,
		Limit:     limit,
	})
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	if len(papers) == 0 {
		return errs.NotFound("no results for %q", in.Query)
	}
	for _, p := range papers {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

func getPaper(ctx context.Context, in paperInput, emit func(*Paper) error) error {
	ref := strings.TrimSpace(in.Ref)
	if ref == "" {
		return errs.Usage("ref is required (e.g. ieee paper 10.1109/cvpr.2016.90)")
	}

	// If it's an Xplore URL, extract article ID for a DOI search via CrossRef
	doi := ref
	if IsIEEEXploreURL(ref) {
		id := ExtractXploreID(ref)
		if id == "" {
			return errs.Usage("could not extract article ID from URL: %s", ref)
		}
		// Try to find the DOI via CrossRef search
		doi = IEEEDoiPrefix + "/" + id
	}

	p, err := in.Client.GetPaper(ctx, doi)
	if err != nil {
		return err
	}
	return emit(p)
}

func topPapers(ctx context.Context, in topInput, emit func(Paper) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	papers, err := in.Client.TopPapers(ctx, in.Subject, limit)
	if err != nil {
		return fmt.Errorf("top papers: %w", err)
	}
	if len(papers) == 0 {
		return errs.NotFound("no papers found")
	}
	for _, p := range papers {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver ---

// Classify turns any accepted input into (uriType, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("ieee: empty input")
	}

	// Full Xplore URL
	if IsIEEEXploreURL(input) {
		xid := ExtractXploreID(input)
		if xid != "" {
			return "paper", xid, nil
		}
	}

	// DOI: starts with 10.1109/
	if strings.HasPrefix(input, "10.1109/") || strings.HasPrefix(input, "https://doi.org/10.1109/") {
		doi := normalizeDOI(input)
		return "paper", doi, nil
	}

	// doi.org URL for any DOI
	if strings.Contains(input, "doi.org/") {
		doi := normalizeDOI(input)
		return "paper", doi, nil
	}

	return "", "", errs.Usage("ieee: unrecognized reference %q (expected DOI or Xplore URL)", input)
}

// Locate returns the canonical URL for a (uriType, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "paper":
		if strings.HasPrefix(id, "10.") {
			return "https://doi.org/" + id, nil
		}
		return fmt.Sprintf("https://%s/document/%s", IEEEHost, id), nil
	default:
		return "", errs.Usage("ieee has no resource type %q", uriType)
	}
}
