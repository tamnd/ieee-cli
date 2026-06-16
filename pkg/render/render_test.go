package render

import (
	"bytes"
	"strings"
	"testing"
)

type rec struct {
	ID    int      `json:"id"`
	Title string   `json:"title"`
	URL   string   `json:"url"`
	HNURL string   `json:"hn_url"`
	Tags  []string `json:"tags"`
}

func renderTo(t *testing.T, f Format, fields []string, noHeader bool, tmpl string, v any) string {
	t.Helper()
	var b bytes.Buffer
	r := New(&b, f, fields, noHeader, tmpl)
	if err := r.Render(v); err != nil {
		t.Fatalf("render %s: %v", f, err)
	}
	return b.String()
}

func sample() []rec {
	return []rec{
		{ID: 1, Title: "First", URL: "https://a.example", HNURL: "https://news.ycombinator.com/item?id=1", Tags: []string{"go", "cli"}},
		{ID: 2, Title: "Second", URL: "", HNURL: "https://news.ycombinator.com/item?id=2"},
	}
}

func TestFormatValid(t *testing.T) {
	for _, f := range []Format{FormatTable, FormatJSON, FormatJSONL, FormatCSV, FormatTSV, FormatURL, FormatRaw} {
		if !f.Valid() {
			t.Errorf("%q should be valid", f)
		}
	}
	if Format("bogus").Valid() {
		t.Error("bogus should be invalid")
	}
}

func TestRenderJSONLOnePerLine(t *testing.T) {
	out := renderTo(t, FormatJSONL, nil, false, "", sample())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], `{"id":1,`) {
		t.Errorf("line0 = %q", lines[0])
	}
}

func TestRenderJSONSingleIsObjectMultiIsArray(t *testing.T) {
	one := renderTo(t, FormatJSON, nil, false, "", []rec{{ID: 9, Title: "X"}})
	if !strings.HasPrefix(strings.TrimSpace(one), "{") {
		t.Errorf("single record should render as object, got %q", one)
	}
	many := renderTo(t, FormatJSON, nil, false, "", sample())
	if !strings.HasPrefix(strings.TrimSpace(many), "[") {
		t.Errorf("multi record should render as array, got %q", many)
	}
}

func TestRenderSingleStructNotSlice(t *testing.T) {
	out := renderTo(t, FormatJSONL, nil, false, "", rec{ID: 5, Title: "solo"})
	if !strings.Contains(out, `"id":5`) {
		t.Errorf("single struct not rendered: %q", out)
	}
}

func TestRenderCSVHeaderAndRows(t *testing.T) {
	out := renderTo(t, FormatCSV, nil, false, "", sample())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "id,title,url,hn_url,tags" {
		t.Errorf("header = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "1,First,") {
		t.Errorf("row1 = %q", lines[1])
	}
	// slice field is joined with ;
	if !strings.Contains(lines[1], "go;cli") {
		t.Errorf("slice field not joined: %q", lines[1])
	}
}

func TestRenderCSVNoHeader(t *testing.T) {
	out := renderTo(t, FormatCSV, nil, true, "", sample())
	if strings.Contains(out, "id,title") {
		t.Errorf("header should be suppressed: %q", out)
	}
}

func TestRenderTSVUsesTabs(t *testing.T) {
	out := renderTo(t, FormatTSV, nil, false, "", sample())
	if !strings.Contains(out, "id\ttitle\t") {
		t.Errorf("tsv header not tab-separated: %q", out)
	}
}

func TestRenderFieldsSelectAndOrder(t *testing.T) {
	out := renderTo(t, FormatCSV, []string{"title", "id"}, false, "", sample())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "title,id" {
		t.Errorf("fields header = %q", lines[0])
	}
	if lines[1] != "First,1" {
		t.Errorf("fields row = %q", lines[1])
	}
}

func TestRenderTableUppercaseHeader(t *testing.T) {
	out := renderTo(t, FormatTable, []string{"id", "title"}, false, "", sample())
	if !strings.Contains(out, "ID") || !strings.Contains(out, "TITLE") {
		t.Errorf("table header not uppercase: %q", out)
	}
}

func TestRenderURLFallsBackToHNURL(t *testing.T) {
	out := renderTo(t, FormatURL, nil, false, "", sample())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "https://a.example" {
		t.Errorf("rec1 url = %q", lines[0])
	}
	if lines[1] != "https://news.ycombinator.com/item?id=2" {
		t.Errorf("rec2 should fall back to hn_url, got %q", lines[1])
	}
}

func TestRenderRawSpaceJoined(t *testing.T) {
	out := renderTo(t, FormatRaw, []string{"id", "title"}, false, "", sample())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "1 First" {
		t.Errorf("raw row = %q", lines[0])
	}
}

func TestRenderTemplatePerRecord(t *testing.T) {
	out := renderTo(t, FormatJSONL, nil, false, "{{.id}}::{{.title}}", sample())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "1::First" {
		t.Errorf("template row = %q", lines[0])
	}
}

func TestRenderTemplateJoinFunc(t *testing.T) {
	out := renderTo(t, FormatJSONL, nil, false, `{{join "," .tags}}`, []rec{{Tags: []string{"a", "b", "c"}}})
	if strings.TrimSpace(out) != "a,b,c" {
		t.Errorf("join template = %q", out)
	}
}

func TestRenderTemplateParseError(t *testing.T) {
	var b bytes.Buffer
	r := New(&b, FormatJSONL, nil, false, "{{.id")
	if err := r.Render(sample()); err == nil {
		t.Error("expected parse error for malformed template")
	}
}

func TestRenderEmptySlice(t *testing.T) {
	out := renderTo(t, FormatTable, nil, false, "", []rec{})
	if out != "" {
		t.Errorf("empty slice should render nothing, got %q", out)
	}
}
