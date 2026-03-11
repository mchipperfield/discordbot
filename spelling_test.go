package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCleanWord(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"hello!", "hello"},
		{"hello,", "hello"},
		{"'hello'", "hello"},
		{"he-llo", "hello"},
		{"he llo", "hello"},
		{"", ""},
		{"123", "123"},
		{"abc123", "abc123"},
		{"...dots...", "dots"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanWord(tt.input)
			if got != tt.want {
				t.Errorf("cleanWord(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLoadSpellingsFromURL(t *testing.T) {
	t.Run("parses valid spelling file", func(t *testing.T) {
		body := "colour color\nflavour flavor\ncentre center\n"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(body))
		}))
		defer srv.Close()

		spellings, err := LoadSpellingsFromURL(srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// map is american -> british
		cases := []struct{ us, uk string }{
			{"color", "colour"},
			{"flavor", "flavour"},
			{"center", "centre"},
		}
		for _, c := range cases {
			if got := spellings[c.us]; got != c.uk {
				t.Errorf("spellings[%q] = %q, want %q", c.us, got, c.uk)
			}
		}
	})

	t.Run("includes custom words", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		spellings, err := LoadSpellingsFromURL(srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if spellings["bitch"] != "bish" {
			t.Errorf("expected custom word bitch -> bish, got %q", spellings["bitch"])
		}
		if spellings["bitches"] != "bishes" {
			t.Errorf("expected custom word bitches -> bishes, got %q", spellings["bitches"])
		}
	})

	t.Run("skips malformed lines", func(t *testing.T) {
		body := "colour color\nbadline\nflavour flavor\n"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(body))
		}))
		defer srv.Close()

		spellings, err := LoadSpellingsFromURL(srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if spellings["color"] != "colour" {
			t.Errorf("expected color -> colour")
		}
		if spellings["flavor"] != "flavour" {
			t.Errorf("expected flavor -> flavour")
		}
		if _, ok := spellings["badline"]; ok {
			t.Errorf("malformed line should not be in map")
		}
	})

	t.Run("returns error on non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := LoadSpellingsFromURL(srv.URL)
		if err == nil {
			t.Fatal("expected error for non-200 status, got nil")
		}
	})

	t.Run("returns error for unreachable URL", func(t *testing.T) {
		_, err := LoadSpellingsFromURL("http://localhost:0/nonexistent")
		if err == nil {
			t.Fatal("expected error for unreachable URL, got nil")
		}
	})
}

func TestBuildSpellingSuggestions(t *testing.T) {
	spellings := map[string]string{
		"color":  "colour",
		"flavor": "flavour",
		"center": "centre",
	}

	tests := []struct {
		name    string
		message string
		want    []string
	}{
		{
			name:    "no american spellings",
			message: "hello world",
			want:    nil,
		},
		{
			name:    "single american spelling",
			message: "I love this color",
			want:    []string{"`color` -> `colour`"},
		},
		{
			name:    "multiple american spellings",
			message: "The color and flavor are great",
			want:    []string{"`color` -> `colour`", "`flavor` -> `flavour`"},
		},
		{
			name:    "word with punctuation",
			message: "Nice color!",
			want:    []string{"`color` -> `colour`"},
		},
		{
			name:    "case insensitive",
			message: "Nice COLOR!",
			want:    []string{"`color` -> `colour`"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSpellingSuggestions(tt.message, spellings)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d suggestions, want %d: %v", len(got), len(tt.want), got)
			}
			for i, s := range got {
				if s != tt.want[i] {
					t.Errorf("suggestion[%d] = %q, want %q", i, s, tt.want[i])
				}
			}
		})
	}
}
