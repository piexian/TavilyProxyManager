package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			name: "research_default_auto_min",
			path: "/research",
			body: `{"input":"latest AI"}`,
			want: 15,
		},
		{
			name: "research_mini_min",
			path: "/research",
			body: `{"input":"latest AI","model":"mini"}`,
			want: 4,
		},
		{
			name: "research_pro_min",
			path: "/research",
			body: `{"input":"latest AI","model":"pro"}`,
			want: 15,
		},
		{
			name: "unknown_endpoint",
			path: "/unknown",
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
		name      string
		body      string
		want      int
		wantFound bool
	}{
		{
			name:      "credits_in_response",
			body:      `{"results":[],"usage":{"credits":3}}`,
			want:      3,
			wantFound: true,
		},
		{
			name:      "fractional_credits",
			body:      `{"results":[],"usage":{"credits":2.5}}`,
			want:      3, // ceil(2.5) = 3
			wantFound: true,
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
			name:      "zero_credits",
			body:      `{"results":[],"usage":{"credits":0}}`,
			want:      0,
			wantFound: true,
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
			got, found := parseCreditsFromResponse([]byte(tt.body))
			if got != tt.want {
				t.Errorf("parseCreditsFromResponse(%q) = %d, want %d", tt.body, got, tt.want)
			}
			if found != tt.wantFound {
				t.Errorf("parseCreditsFromResponse(%q) found = %v, want %v", tt.body, found, tt.wantFound)
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

func TestResolveCredits_PreservesZeroUsage(t *testing.T) {
	t.Parallel()

	respBody := []byte(`{"results":[],"usage":{"credits":0}}`)
	req := ProxyRequest{Path: "/search", Body: []byte(`{"query":"test","search_depth":"advanced"}`)}

	got := resolveCredits(respBody, req)
	if got != 0 {
		t.Errorf("resolveCredits with usage.credits=0 = %d, want 0", got)
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

func TestTavilyProxy_MapInjectsUsageForAccountingAndStripsDefaultResponse(t *testing.T) {
	t.Parallel()

	var upstreamReq map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := bearerToken(r); got != "tvly-test" {
			t.Fatalf("bearer token = %q, want tvly-test", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamReq); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":["https://example.com"],"usage":{"credits":0},"request_id":"req-map"}`))
	}))
	t.Cleanup(upstream.Close)

	ctx, keys, proxy := newTavilyProxyTestDeps(t, upstream.URL)
	key, err := keys.Create(ctx, "tvly-test", "test", 1000)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	resp, err := proxy.Do(ctx, ProxyRequest{
		Method:      http.MethodPost,
		Path:        "/map",
		Body:        []byte(`{"url":"https://example.com"}`),
		ContentType: "application/json",
		ClientIP:    "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if upstreamReq["include_usage"] != true {
		t.Fatalf("upstream include_usage = %v, want true", upstreamReq["include_usage"])
	}

	var out map[string]any
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if _, ok := out["usage"]; ok {
		t.Fatalf("response leaked usage field: %s", string(resp.Body))
	}

	gotKey, err := keys.Get(ctx, key.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if gotKey.UsedQuota != 0 {
		t.Fatalf("used quota = %d, want 0", gotKey.UsedQuota)
	}
	if gotKey.LastUsedAt == nil {
		t.Fatal("last_used_at was not updated")
	}
}

func TestTavilyProxy_CrawlPreservesUsageWhenClientRequested(t *testing.T) {
	t.Parallel()

	var upstreamReq map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamReq); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[],"usage":{"credits":4},"request_id":"req-crawl"}`))
	}))
	t.Cleanup(upstream.Close)

	ctx, keys, proxy := newTavilyProxyTestDeps(t, upstream.URL)
	key, err := keys.Create(ctx, "tvly-test", "test", 1000)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	resp, err := proxy.Do(ctx, ProxyRequest{
		Method:      http.MethodPost,
		Path:        "/crawl",
		Body:        []byte(`{"url":"https://example.com","include_usage":true}`),
		ContentType: "application/json",
		ClientIP:    "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if upstreamReq["include_usage"] != true {
		t.Fatalf("upstream include_usage = %v, want true", upstreamReq["include_usage"])
	}

	var out map[string]any
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if _, ok := out["usage"]; !ok {
		t.Fatalf("response missing usage field: %s", string(resp.Body))
	}

	gotKey, err := keys.Get(ctx, key.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if gotKey.UsedQuota != 4 {
		t.Fatalf("used quota = %d, want 4", gotKey.UsedQuota)
	}
}

func TestTavilyProxy_SearchInjectsUsageForAccountingAndStripsDefaultResponse(t *testing.T) {
	t.Parallel()

	var upstreamReq map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&upstreamReq); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[],"usage":{"credits":2},"request_id":"req-search"}`))
	}))
	t.Cleanup(upstream.Close)

	ctx, keys, proxy := newTavilyProxyTestDeps(t, upstream.URL)
	key, err := keys.Create(ctx, "tvly-test", "test", 1000)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	resp, err := proxy.Do(ctx, ProxyRequest{
		Method:      http.MethodPost,
		Path:        "/search",
		Body:        []byte(`{"query":"test","search_depth":"advanced"}`),
		ContentType: "application/json",
		ClientIP:    "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if upstreamReq["include_usage"] != true {
		t.Fatalf("upstream include_usage = %v, want true", upstreamReq["include_usage"])
	}

	var out map[string]any
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if _, ok := out["usage"]; ok {
		t.Fatalf("response leaked usage field: %s", string(resp.Body))
	}

	gotKey, err := keys.Get(ctx, key.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if gotKey.UsedQuota != 2 {
		t.Fatalf("used quota = %d, want 2", gotKey.UsedQuota)
	}
}

func TestTavilyProxy_Research201BillsMinimumCredits(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/research" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"request_id":"abc","status":"pending","input":"latest AI","model":"mini","response_time":1}`))
	}))
	t.Cleanup(upstream.Close)

	ctx, keys, proxy := newTavilyProxyTestDeps(t, upstream.URL)
	key, err := keys.Create(ctx, "tvly-test", "test", 1000)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}

	resp, err := proxy.Do(ctx, ProxyRequest{
		Method:      http.MethodPost,
		Path:        "/research",
		Body:        []byte(`{"input":"latest AI","model":"mini"}`),
		ContentType: "application/json",
		ClientIP:    "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("proxy request: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	gotKey, err := keys.Get(ctx, key.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	if gotKey.UsedQuota != 4 {
		t.Fatalf("used quota = %d, want 4 (research mini minimum)", gotKey.UsedQuota)
	}
}
