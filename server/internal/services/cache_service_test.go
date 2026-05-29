package services

import (
	"testing"
)

func TestBuildCacheKey_DifferentParamsProduceDifferentKeys(t *testing.T) {
	t.Parallel()

	s := &CacheService{}

	tests := []struct {
		name   string
		body1  string
		body2  string
		wantEq bool
	}{
		{
			name:   "same_query_same_key",
			body1:  `{"query":"golang"}`,
			body2:  `{"query":"golang"}`,
			wantEq: true,
		},
		{
			name:   "different_query_different_key",
			body1:  `{"query":"golang"}`,
			body2:  `{"query":"python"}`,
			wantEq: false,
		},
		{
			name:   "time_range_changes_key",
			body1:  `{"query":"news","time_range":"day"}`,
			body2:  `{"query":"news","time_range":"month"}`,
			wantEq: false,
		},
		{
			name:   "country_changes_key",
			body1:  `{"query":"hotels","country":"united states"}`,
			body2:  `{"query":"hotels","country":"japan"}`,
			wantEq: false,
		},
		{
			name:   "search_depth_changes_key",
			body1:  `{"query":"test","search_depth":"basic"}`,
			body2:  `{"query":"test","search_depth":"advanced"}`,
			wantEq: false,
		},
		{
			name:   "include_answer_changes_key",
			body1:  `{"query":"test","include_answer":false}`,
			body2:  `{"query":"test","include_answer":true}`,
			wantEq: false,
		},
		{
			name:   "api_key_ignored",
			body1:  `{"query":"test","api_key":"tvly-aaa"}`,
			body2:  `{"query":"test","api_key":"tvly-bbb"}`,
			wantEq: true,
		},
		{
			name:   "apiKey_ignored",
			body1:  `{"query":"test","apiKey":"tvly-aaa"}`,
			body2:  `{"query":"test","apiKey":"tvly-bbb"}`,
			wantEq: true,
		},
		{
			name:   "include_usage_ignored",
			body1:  `{"query":"test","include_usage":false}`,
			body2:  `{"query":"test","include_usage":true}`,
			wantEq: true,
		},
		{
			name:   "chunks_per_source_changes_key",
			body1:  `{"query":"test","chunks_per_source":1}`,
			body2:  `{"query":"test","chunks_per_source":3}`,
			wantEq: false,
		},
		{
			name:   "start_date_changes_key",
			body1:  `{"query":"news"}`,
			body2:  `{"query":"news","start_date":"2025-01-01"}`,
			wantEq: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			key1, _ := s.BuildCacheKey([]byte(tt.body1))
			key2, _ := s.BuildCacheKey([]byte(tt.body2))

			if (key1 == key2) != tt.wantEq {
				t.Errorf("keys equal=%v, want equal=%v\nkey1=%s\nkey2=%s", key1 == key2, tt.wantEq, key1[:min(16, len(key1))], key2[:min(16, len(key2))])
			}
		})
	}
}

func TestBuildCacheKey_ExtractsQuery(t *testing.T) {
	t.Parallel()
	s := &CacheService{}

	_, query := s.BuildCacheKey([]byte(`{"query":"hello world","search_depth":"basic"}`))
	if query != "hello world" {
		t.Errorf("query = %q, want %q", query, "hello world")
	}

	_, query = s.BuildCacheKey([]byte(`{}`))
	if query != "" {
		t.Errorf("query = %q, want empty", query)
	}

	_, query = s.BuildCacheKey([]byte(`not json`))
	if query != "" {
		t.Errorf("query = %q, want empty for invalid json", query)
	}
}
