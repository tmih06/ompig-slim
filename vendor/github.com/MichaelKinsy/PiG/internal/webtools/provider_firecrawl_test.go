package webtools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestFirecrawlResponseShapes(t *testing.T) {
	t.Parallel()
	const row = `{"url":"https://example.com/a","title":"A"}`
	const fallback = `"results":[` + row + `]`
	for name, body := range map[string]string{
		"flat":                `{"data":[` + row + `]}`,
		"grouped":             `{"data":{"web":[` + row + `]}}`,
		"missing":             `{` + fallback + `}`,
		"null":                `{"data":null,` + fallback + `}`,
		"empty array":         `{"data":[],` + fallback + `}`,
		"empty group":         `{"data":{"web":[]},` + fallback + `}`,
		"invalid array":       `{"data":[` + row + `,{"title":false}],` + fallback + `}`,
		"invalid group":       `{"data":{"web":[{"title":false}]},` + fallback + `}`,
		"unexpected data":     `{"data":false,` + fallback + `}`,
		"data takes priority": `{"data":[` + row + `],"results":[{"title":"ignored"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var response firecrawlResponse
			if err := json.Unmarshal([]byte(body), &response); err != nil {
				t.Fatal(err)
			}
			want := []firecrawlItem{{URL: "https://example.com/a", Title: "A"}}
			if got := response.webResults(); !reflect.DeepEqual(got, want) {
				t.Fatalf("webResults() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestFirecrawlOptionalStrings(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"missing": `{"data":[{"url":"https://example.com/a"},{"title":"no URL"}]}`,
		"null":    `{"id":null,"error":null,"data":[{"url":"https://example.com/a","title":null,"description":null,"snippet":null,"markdown":null},{"url":null}]}`,
		"empty":   `{"id":"","error":"","data":[{"url":"https://example.com/a","title":"","description":"","snippet":"","markdown":""},{"url":""}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			result, err := newFirecrawl(server.URL).Search(context.Background(), SearchParams{Query: "x", Limit: 1})
			if err != nil {
				t.Fatal(err)
			}
			want := []SearchSource{{URL: "https://example.com/a", Title: "https://example.com/a"}}
			if result.RequestID != "" || !reflect.DeepEqual(result.Sources, want) {
				t.Fatalf("result = %+v, want sources %+v and no request ID", result, want)
			}
		})
	}
}

func TestFirecrawlNullErrorUsesDefaultMessage(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"success":false,"error":null}`)
	}))
	defer server.Close()
	_, err := newFirecrawl(server.URL).Search(context.Background(), SearchParams{Query: "x", Limit: 1})
	var searchErr *SearchError
	if !errors.As(err, &searchErr) || searchErr.Message != "Firecrawl request failed" {
		t.Fatalf("error = %v", err)
	}
}
