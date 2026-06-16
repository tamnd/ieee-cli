package ieee

import "strings"

// Author is one paper author from CrossRef.
type Author struct {
	Given  string `json:"given"`
	Family string `json:"family"`
}

// String formats an Author as "Family, G."
func (a Author) String() string {
	if a.Given == "" {
		return a.Family
	}
	// Use first letter of given name as initial
	initial := string([]rune(a.Given)[0])
	return a.Family + ", " + initial + "."
}

// Paper is one IEEE paper record assembled from CrossRef metadata.
type Paper struct {
	DOI        string `json:"doi"`
	Title      string `json:"title"`
	AuthorsStr string `json:"authors"`   // formatted author list for display
	Year       int    `json:"year"`
	Venue      string `json:"venue"`     // journal or conference name
	Type       string `json:"type"`      // journal-article, proceedings-article, etc.
	Abstract   string `json:"abstract"`
	Citations  int    `json:"citations"` // CrossRef is-referenced-by-count
	URL        string `json:"url"`       // https://doi.org/<doi>
}

// FormatAuthors formats a slice of Author into a display string.
// More than 5 authors: "A; B; C; D; E; et al."
func FormatAuthors(authors []Author) string {
	if len(authors) == 0 {
		return ""
	}
	max := len(authors)
	trunc := false
	if max > 5 {
		max = 5
		trunc = true
	}
	parts := make([]string, max)
	for i := 0; i < max; i++ {
		parts[i] = authors[i].String()
	}
	s := strings.Join(parts, "; ")
	if trunc {
		s += "; et al."
	}
	return s
}

// StripJATS removes JATS XML markup from a CrossRef abstract.
func StripJATS(s string) string {
	// Remove opening JATS tags: <jats:p>, <jats:bold>, etc.
	out := jatsTagRE.ReplaceAllString(s, "")
	// Collapse whitespace
	fields := strings.Fields(out)
	return strings.Join(fields, " ")
}
