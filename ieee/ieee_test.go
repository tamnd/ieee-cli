package ieee

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockCrossRefResponse builds a CrossRef works response JSON for testing.
func mockCrossRefResponse(items []crossRefWork) string {
	type msg struct {
		TotalResults int            `json:"total-results"`
		Items        []crossRefWork `json:"items"`
	}
	type resp struct {
		Status  string `json:"status"`
		Message msg    `json:"message"`
	}
	r := resp{Status: "ok", Message: msg{TotalResults: len(items), Items: items}}
	b, _ := json.Marshal(r)
	return string(b)
}

func mockWork(doi, title string, year int, venue string) crossRefWork {
	return crossRefWork{
		DOI:   doi,
		Title: []string{title},
		Author: []crAuthor{
			{Given: "First", Family: "Author", Sequence: "first"},
			{Given: "Second", Family: "Coauthor", Sequence: "additional"},
		},
		Published: struct {
			DateParts [][]int `json:"date-parts"`
		}{DateParts: [][]int{{year}}},
		ContainerTitle:      []string{venue},
		Type:                "proceedings-article",
		Abstract:            "<jats:p>This is the abstract.</jats:p>",
		IsReferencedByCount: 42,
	}
}

func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	cfg := DefaultConfig()
	cfg.CrossRefBaseURL = srv.URL
	cfg.Rate = 0
	return NewClient(cfg)
}

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte(`{"status":"ok","message":{"total-results":0,"items":[]}}`))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "status") {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.CrossRefBaseURL = srv.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := NewClient(cfg)

	start := time.Now()
	_, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestSearchPapers(t *testing.T) {
	work := mockWork("10.1109/cvpr.2016.90", "Deep Residual Learning", 2016, "IEEE CVPR")
	respJSON := mockCrossRefResponse([]crossRefWork{work})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respJSON))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	papers, err := c.SearchPapers(context.Background(), "deep learning", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(papers) != 1 {
		t.Fatalf("got %d papers, want 1", len(papers))
	}
	p := papers[0]
	if p.DOI != "10.1109/cvpr.2016.90" {
		t.Errorf("DOI = %q", p.DOI)
	}
	if p.Title != "Deep Residual Learning" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.Year != 2016 {
		t.Errorf("Year = %d", p.Year)
	}
	if p.Venue != "IEEE CVPR" {
		t.Errorf("Venue = %q", p.Venue)
	}
	if !strings.Contains(p.Abstract, "This is the abstract") {
		t.Errorf("Abstract should be stripped of JATS: %q", p.Abstract)
	}
	if p.Citations != 42 {
		t.Errorf("Citations = %d", p.Citations)
	}
	if p.URL != "https://doi.org/10.1109/cvpr.2016.90" {
		t.Errorf("URL = %q", p.URL)
	}
}

func TestGetPaper(t *testing.T) {
	work := mockWork("10.1109/tnn.2019.1", "LSTM Paper", 2019, "IEEE Transactions on Neural Networks")
	type single struct {
		Status  string       `json:"status"`
		Message crossRefWork `json:"message"`
	}
	b, _ := json.Marshal(single{Status: "ok", Message: work})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	c := testClient(t, srv)
	p, err := c.GetPaper(context.Background(), "10.1109/tnn.2019.1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "LSTM Paper" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.Year != 2019 {
		t.Errorf("Year = %d", p.Year)
	}
}

func TestTopPapers(t *testing.T) {
	work := mockWork("10.1109/top.2020.1", "Top Paper", 2020, "IEEE TPAMI")
	work.IsReferencedByCount = 9999
	respJSON := mockCrossRefResponse([]crossRefWork{work})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "sort=is-referenced-by-count") {
			t.Error("top papers request should sort by citation count")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respJSON))
	}))
	defer srv.Close()

	c := testClient(t, srv)
	papers, err := c.TopPapers(context.Background(), "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(papers) != 1 {
		t.Fatalf("got %d papers, want 1", len(papers))
	}
	if papers[0].Citations != 9999 {
		t.Errorf("Citations = %d", papers[0].Citations)
	}
}

func TestStripJATS(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"<jats:p>Hello world.</jats:p>", "Hello world."},
		{"<jats:bold>Bold</jats:bold> text", "Bold text"},
		{"<jats:p>One. <jats:italic>Two.</jats:italic></jats:p>", "One. Two."},
		{"Plain text", "Plain text"},
	}
	for _, tc := range cases {
		got := StripJATS(tc.in)
		if got != tc.want {
			t.Errorf("StripJATS(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatAuthors(t *testing.T) {
	cases := []struct {
		authors []Author
		want    string
	}{
		{nil, ""},
		{[]Author{{Given: "Kaiming", Family: "He"}}, "He, K."},
		{[]Author{
			{Given: "K", Family: "He"},
			{Given: "X", Family: "Zhang"},
		}, "He, K.; Zhang, X."},
		{[]Author{
			{Given: "A", Family: "One"},
			{Given: "B", Family: "Two"},
			{Given: "C", Family: "Three"},
			{Given: "D", Family: "Four"},
			{Given: "E", Family: "Five"},
			{Given: "F", Family: "Six"},
		}, "One, A.; Two, B.; Three, C.; Four, D.; Five, E.; et al."},
	}
	for _, tc := range cases {
		got := FormatAuthors(tc.authors)
		if got != tc.want {
			t.Errorf("FormatAuthors(%v) = %q, want %q", tc.authors, got, tc.want)
		}
	}
}

func TestExtractXploreID(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://ieeexplore.ieee.org/document/7780459", "7780459"},
		{"https://ieeexplore.ieee.org/document/7780459/", "7780459"},
		{"https://ieeexplore.ieee.org/abstract/document/7780459", "7780459"},
		{"https://example.com", ""},
	}
	for _, tc := range cases {
		got := ExtractXploreID(tc.url)
		if got != tc.want {
			t.Errorf("ExtractXploreID(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestPaceRespected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.CrossRefBaseURL = srv.URL
	cfg.Rate = 100 * time.Millisecond
	c := NewClient(cfg)

	start := time.Now()
	for i := 0; i < 3; i++ {
		_, err := c.Get(context.Background(), srv.URL)
		if err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(start) < 200*time.Millisecond {
		t.Error("3 requests should take >= 200ms with 100ms rate")
	}
}
