package services

import (
	"testing"
)

func TestEstimateCreditsFromRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		body string
		want int
	}{
		{
			name: "search_basic",
			path: "/search",
			body: `{"query":"test","search_depth":"basic"}`,
			want: 1,
		},
		{
			name: "search_advanced",
			path: "/search",
			body: `{"query":"test","search_depth":"advanced"}`,
			want: 2,
		},
		{
			name: "search_default_depth",
			path: "/search",
			body: `{"query":"test"}`,
			want: 1,
		},
		{
			name: "search_fast",
			path: "/search",
			body: `{"query":"test","search_depth":"fast"}`,
			want: 1,
		},
		{
			name: "search_ultra_fast",
			path: "/search",
			body: `{"query":"test","search_depth":"ultra-fast"}`,
			want: 1,
		},
		{
			name: "extract_basic_10_urls",
			path: "/extract",
			body: `{"urls":["a","b","c","d","e","f","g","h","i","j"]}`,
			want: 2, // ceil(10/5) * 1 = 2
		},
		{
			name: "extract_advanced_10_urls",
			path: "/extract",
			body: `{"urls":["a","b","c","d","e","f","g","h","i","j"],"extract_depth":"advanced"}`,
			want: 4, // ceil(10/5) * 2 = 4
		},
		{
			name: "extract_basic_3_urls",
			path: "/extract",
			body: `{"urls":["a","b","c"]}`,
			want: 1, // ceil(3/5) * 1 = 1
		},
		{
			name: "extract_advanced_3_urls",
			path: "/extract",
			body: `{"urls":["a","b","c"],"extract_depth":"advanced"}`,
			want: 2, // ceil(3/5) * 2 = 2
		},
		{
			name: "extract_no_urls",
			path: "/extract",
			body: `{"urls":[]}`,
			want: 1, // fallback to 1 when no URLs
		},
		{
			name: "crawl_conservative",
			path: "/crawl",
			body: `{"url":"https://example.com"}`,
			want: 1,
		},
		{
			name: "map_conservative",
			path: "/map",
			body: `{"url":"https://example.com"}`,
			want: 1,
		},
		{
			name: "unknown_endpoint",
			path: "/research",
			body: `{}`,
			want: 1,
		},
		{
			name: "empty_body",
			path: "/search",
			body: ``,
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := ProxyRequest{Path: tt.path, Body: []byte(tt.body)}
			got := estimateCreditsFromRequest(req)
			if got != tt.want {
				t.Errorf("estimateCreditsFromRequest(%q, %q) = %d, want %d", tt.path, tt.body, got, tt.want)
			}
		})
	}
}

func TestParseCreditsFromResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want int
	}{
		{
			name: "credits_in_response",
			body: `{"results":[],"usage":{"credits":3}}`,
			want: 3,
		},
		{
			name: "fractional_credits",
			body: `{"results":[],"usage":{"credits":2.5}}`,
			want: 3, // ceil(2.5) = 3
		},
		{
			name: "no_usage_field",
			body: `{"results":[],"answer":"hello"}`,
			want: 0,
		},
		{
			name: "usage_without_credits",
			body: `{"results":[],"usage":{"total":5}}`,
			want: 0,
		},
		{
			name: "zero_credits",
			body: `{"results":[],"usage":{"credits":0}}`,
			want: 0,
		},
		{
			name: "invalid_json",
			body: `{not json}`,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseCreditsFromResponse([]byte(tt.body))
			if got != tt.want {
				t.Errorf("parseCreditsFromResponse(%q) = %d, want %d", tt.body, got, tt.want)
			}
		})
	}
}

func TestResolveCredits_PrefersResponse(t *testing.T) {
	t.Parallel()

	// When response has usage.credits, use it regardless of request
	respBody := []byte(`{"results":[],"usage":{"credits":3}}`)
	req := ProxyRequest{Path: "/search", Body: []byte(`{"query":"test","search_depth":"basic"}`)}

	got := resolveCredits(respBody, req)
	if got != 3 {
		t.Errorf("resolveCredits with usage.credits = %d, want 3", got)
	}
}

func TestResolveCredits_FallsBackToEstimate(t *testing.T) {
	t.Parallel()

	// When response has no usage.credits, fall back to estimate
	respBody := []byte(`{"results":[],"answer":"hello"}`)
	req := ProxyRequest{Path: "/search", Body: []byte(`{"query":"test","search_depth":"advanced"}`)}

	got := resolveCredits(respBody, req)
	if got != 2 {
		t.Errorf("resolveCredits fallback for advanced search = %d, want 2", got)
	}
}
