package ieee

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// jatsTagRE matches JATS XML tags like <jats:p>, </jats:bold>, etc.
var jatsTagRE = regexp.MustCompile(`</?jats:[a-zA-Z0-9]+[^>]*>`)

// crossRefResponse is the top-level CrossRef API response envelope.
type crossRefResponse struct {
	Status  string `json:"status"`
	Message struct {
		TotalResults int             `json:"total-results"`
		Items        []crossRefWork  `json:"items"`
	} `json:"message"`
}

// crossRefSingle is the CrossRef response for a single work.
type crossRefSingle struct {
	Status  string       `json:"status"`
	Message crossRefWork `json:"message"`
}

// crossRefWork is one CrossRef work object.
type crossRefWork struct {
	DOI       string     `json:"DOI"`
	Title     []string   `json:"title"`
	Author    []crAuthor `json:"author"`
	Published struct {
		DateParts [][]int `json:"date-parts"`
	} `json:"published"`
	ContainerTitle       []string `json:"container-title"`
	Type                 string   `json:"type"`
	Abstract             string   `json:"abstract"`
	IsReferencedByCount  int      `json:"is-referenced-by-count"`
}

type crAuthor struct {
	Given    string `json:"given"`
	Family   string `json:"family"`
	Sequence string `json:"sequence"`
}

// toAuthors converts CrossRef authors to our Author slice.
func toAuthors(crs []crAuthor) []Author {
	out := make([]Author, 0, len(crs))
	for _, a := range crs {
		out = append(out, Author{Given: a.Given, Family: a.Family})
	}
	return out
}

// workToPaper converts a CrossRef work to a Paper.
func workToPaper(w crossRefWork) Paper {
	// Title
	title := ""
	if len(w.Title) > 0 {
		title = w.Title[0]
	}

	// Year
	year := 0
	if len(w.Published.DateParts) > 0 && len(w.Published.DateParts[0]) > 0 {
		year = w.Published.DateParts[0][0]
	}

	// Venue
	venue := ""
	if len(w.ContainerTitle) > 0 {
		venue = w.ContainerTitle[0]
	}

	// Type normalization
	docType := normalizeType(w.Type)

	// Authors
	authors := toAuthors(w.Author)
	authorsStr := FormatAuthors(authors)

	// Abstract: strip JATS tags
	abstract := ""
	if w.Abstract != "" {
		abstract = StripJATS(w.Abstract)
	}

	// URL
	paperURL := ""
	if w.DOI != "" {
		paperURL = "https://doi.org/" + w.DOI
	}

	return Paper{
		DOI:        w.DOI,
		Title:      title,
		AuthorsStr: authorsStr,
		Year:       year,
		Venue:      venue,
		Type:       docType,
		Abstract:   abstract,
		Citations:  w.IsReferencedByCount,
		URL:        paperURL,
	}
}

func normalizeType(t string) string {
	switch t {
	case "journal-article":
		return "journal"
	case "proceedings-article":
		return "conference"
	case "standard":
		return "standard"
	case "book-chapter":
		return "book-chapter"
	default:
		return t
	}
}

// SearchPapers queries CrossRef for IEEE papers matching query.
func (c *Client) SearchPapers(ctx context.Context, query string, opts SearchOptions) ([]Paper, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("rows", fmt.Sprintf("%d", opts.Limit))
	params.Set("sort", "relevance")

	// Build filter
	filters := []string{"prefix:" + IEEEDoiPrefix}
	if opts.YearStart > 0 {
		filters = append(filters, fmt.Sprintf("from-pub-date:%d", opts.YearStart))
	}
	if opts.YearEnd > 0 {
		filters = append(filters, fmt.Sprintf("until-pub-date:%d", opts.YearEnd))
	}
	if typeFilter := typeToFilter(opts.DocType); typeFilter != "" {
		filters = append(filters, typeFilter)
	}
	params.Set("filter", strings.Join(filters, ","))

	// Select fields to reduce payload size
	params.Set("select", "DOI,title,author,published,abstract,container-title,type")

	if c.cfg.Mailto != "" {
		params.Set("mailto", c.cfg.Mailto)
	}

	rawURL := c.cfg.CrossRefBaseURL + "/works?" + params.Encode()
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("search papers: %w", err)
	}

	var resp crossRefResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse search response: %w", err)
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("crossref status: %s", resp.Status)
	}

	papers := make([]Paper, 0, len(resp.Message.Items))
	for _, w := range resp.Message.Items {
		papers = append(papers, workToPaper(w))
	}
	return papers, nil
}

// SearchOptions holds optional parameters for SearchPapers.
type SearchOptions struct {
	YearStart int
	YearEnd   int
	DocType   string // "journal", "conference", "standard", ""
	Limit     int
}

// GetPaper fetches a single paper by DOI.
func (c *Client) GetPaper(ctx context.Context, doi string) (*Paper, error) {
	doi = normalizeDOI(doi)
	rawURL := c.cfg.CrossRefBaseURL + "/works/" + url.PathEscape(doi)
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("get paper %s: %w", doi, err)
	}

	var resp crossRefSingle
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse paper response: %w", err)
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("crossref status: %s", resp.Status)
	}

	p := workToPaper(resp.Message)
	return &p, nil
}

// TopPapers fetches the most-cited IEEE papers via CrossRef.
func (c *Client) TopPapers(ctx context.Context, subject string, limit int) ([]Paper, error) {
	params := url.Values{}
	params.Set("filter", "prefix:"+IEEEDoiPrefix)
	params.Set("rows", fmt.Sprintf("%d", limit))
	params.Set("sort", "is-referenced-by-count")
	params.Set("order", "desc")
	params.Set("select", "DOI,title,author,published,abstract,container-title,type,is-referenced-by-count")

	if subject != "" {
		params.Set("query", subjectQuery(subject))
	}
	if c.cfg.Mailto != "" {
		params.Set("mailto", c.cfg.Mailto)
	}

	rawURL := c.cfg.CrossRefBaseURL + "/works?" + params.Encode()
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("top papers: %w", err)
	}

	var resp crossRefResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse top papers: %w", err)
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf("crossref status: %s", resp.Status)
	}

	papers := make([]Paper, 0, len(resp.Message.Items))
	for _, w := range resp.Message.Items {
		papers = append(papers, workToPaper(w))
	}
	return papers, nil
}

// normalizeDOI accepts a DOI in various forms and returns the canonical DOI string.
// Handles:
//   - Bare DOI: "10.1109/cvpr.2016.90"
//   - https://doi.org/10.1109/... prefix
//   - https://ieeexplore.ieee.org/document/<id> (returns as-is; caller handles)
func normalizeDOI(ref string) string {
	ref = strings.TrimSpace(ref)
	// Strip doi.org prefix
	if strings.HasPrefix(ref, "https://doi.org/") {
		return strings.TrimPrefix(ref, "https://doi.org/")
	}
	if strings.HasPrefix(ref, "http://doi.org/") {
		return strings.TrimPrefix(ref, "http://doi.org/")
	}
	// Strip Xplore URL - extract the document ID, not a real DOI
	// Caller must handle this case before calling normalizeDOI
	return ref
}

// typeToFilter maps CLI type names to CrossRef filter strings.
func typeToFilter(t string) string {
	switch strings.ToLower(t) {
	case "journal":
		return "type:journal-article"
	case "conference":
		return "type:proceedings-article"
	case "standard":
		return "type:standard"
	default:
		return ""
	}
}

// subjectQuery maps subject slugs to CrossRef query additions.
func subjectQuery(subject string) string {
	switch strings.ToLower(subject) {
	case "signal-processing":
		return "signal processing"
	case "computer-vision":
		return "computer vision"
	case "machine-learning":
		return "machine learning"
	case "networking":
		return "computer network"
	case "robotics":
		return "robotics"
	case "power-systems":
		return "power system"
	case "communications":
		return "wireless communication"
	default:
		return subject
	}
}

// IsIEEEXploreURL reports whether ref is an ieeexplore.ieee.org URL.
func IsIEEEXploreURL(ref string) bool {
	return strings.Contains(ref, "ieeexplore.ieee.org")
}

// ExtractXploreID extracts the numeric article ID from an IEEE Xplore URL.
// "https://ieeexplore.ieee.org/document/7780459" -> "7780459"
func ExtractXploreID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, p := range parts {
		if p == "document" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
