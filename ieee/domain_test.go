package ieee

import (
	"strings"
	"testing"

	"github.com/tamnd/any-cli/kit"
)

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "ieee" {
		t.Errorf("Scheme = %q, want ieee", info.Scheme)
	}
	if len(info.Hosts) == 0 {
		t.Error("Hosts should not be empty")
	}
	if info.Identity.Binary != "ieee" {
		t.Errorf("Binary = %q, want ieee", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in      string
		wantTyp string
		wantID  string
		wantErr bool
	}{
		{"10.1109/cvpr.2016.90", "paper", "10.1109/cvpr.2016.90", false},
		{"https://doi.org/10.1109/cvpr.2016.90", "paper", "10.1109/cvpr.2016.90", false},
		{"https://ieeexplore.ieee.org/document/7780459", "paper", "7780459", false},
		{"", "", "", true},
		{"not-a-doi", "", "", true},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Classify(%q) expected error, got (%q,%q)", tc.in, typ, id)
			}
			continue
		}
		if err != nil {
			t.Errorf("Classify(%q) error = %v", tc.in, err)
			continue
		}
		if typ != tc.wantTyp || id != tc.wantID {
			t.Errorf("Classify(%q) = (%q,%q), want (%q,%q)", tc.in, typ, id, tc.wantTyp, tc.wantID)
		}
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		typ     string
		id      string
		want    string
		wantErr bool
	}{
		{"paper", "10.1109/cvpr.2016.90", "https://doi.org/10.1109/cvpr.2016.90", false},
		{"paper", "7780459", "https://ieeexplore.ieee.org/document/7780459", false},
		{"bogus", "x", "", true},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.typ, tc.id)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Locate(%q,%q) expected error", tc.typ, tc.id)
			}
			continue
		}
		if err != nil {
			t.Errorf("Locate(%q,%q) error = %v", tc.typ, tc.id, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Locate(%q,%q) = %q, want %q", tc.typ, tc.id, got, tc.want)
		}
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.ResolveOn("ieee", "10.1109/cvpr.2016.90")
	if err != nil {
		t.Fatalf("ResolveOn: %v", err)
	}
	if !strings.Contains(got.String(), "ieee://paper/") {
		t.Errorf("ResolveOn = %q, want ieee://paper/...", got.String())
	}
}
