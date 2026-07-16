package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tavily-proxy/server/internal/services"
)

type Dependencies struct {
	MasterKey  *services.MasterKeyService
	AccessKeys *services.AccessKeyService
	Proxy      *services.TavilyProxy
	Stats      *services.StatsService
	Stateless  bool
	SessionTTL time.Duration
}

type contextKey int

const (
	ctxClientIP contextKey = iota
	ctxAccessKeyID
	ctxAccessKeyAlias
)

type Handlers struct {
	Streamable http.Handler
	SSE        http.Handler
}

func NewHandler(deps Dependencies) Handlers {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "tavily-proxy-mcp",
		Version: "0.1.0",
	}, nil)

	addProxyTool(server, deps.Proxy, &mcp.Tool{
		Name:        "tavily-search",
		Description: "Execute a search query using Tavily Search (via Tavily Proxy Pool). Returns ranked results and optional answer/raw_content/images/usage.",
		InputSchema: tavilySearchInputSchema,
	}, http.MethodPost, "/search")
	addProxyTool(server, deps.Proxy, &mcp.Tool{
		Name:        "tavily-extract",
		Description: "Extract structured content from URLs (via Tavily Proxy Pool)",
		InputSchema: tavilyExtractInputSchema,
	}, http.MethodPost, "/extract")
	addProxyTool(server, deps.Proxy, &mcp.Tool{
		Name:        "tavily-crawl",
		Description: "Crawl a website starting from a root URL (via Tavily Proxy Pool)",
		InputSchema: tavilyCrawlInputSchema,
	}, http.MethodPost, "/crawl")
	addProxyTool(server, deps.Proxy, &mcp.Tool{
		Name:        "tavily-map",
		Description: "Map a website's URL structure (via Tavily Proxy Pool)",
		InputSchema: tavilyMapInputSchema,
	}, http.MethodPost, "/map")
	addProxyTool(server, deps.Proxy, &mcp.Tool{
		Name:        "tavily-research",
		Description: "Create a Tavily Research task (via Tavily Proxy Pool). Returns request_id and status; poll with tavily-research-get until completed/failed. Streaming (stream=true) is not supported through this tool.",
		InputSchema: tavilyResearchInputSchema,
	}, http.MethodPost, "/research")
	addResearchGetTool(server, deps.Proxy, &mcp.Tool{
		Name:        "tavily-research-get",
		Description: "Get status/results of a Tavily Research task by request_id (via Tavily Proxy Pool).",
		InputSchema: tavilyResearchGetInputSchema,
	})
	addUsageTool(server, deps.Stats, &mcp.Tool{
		Name:        "tavily-usage",
		Description: "Get aggregated usage/quota info from local key statistics",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
	})

	getServer := func(_ *http.Request) *mcp.Server {
		return server
	}

	streamable := mcp.NewStreamableHTTPHandler(getServer, &mcp.StreamableHTTPOptions{
		Stateless:      deps.Stateless,
		SessionTimeout: deps.SessionTTL,
	})

	sse := mcp.NewSSEHandler(getServer, nil)

	authWrap := func(base http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := parseBearerToken(r.Header.Get("Authorization"))

			var accessKeyID uint
			var accessKeyAlias string

			if !deps.MasterKey.Authenticate(token) {
				if deps.AccessKeys == nil {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				ak, ok := deps.AccessKeys.Authenticate(token)
				if !ok {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				accessKeyID = ak.ID
				accessKeyAlias = ak.Alias
			}

			// Inject client IP and access key info into context.
			clientIP := r.RemoteAddr
			if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
				clientIP = strings.SplitN(ip, ",", 2)[0]
			} else if ip := r.Header.Get("X-Real-Ip"); ip != "" {
				clientIP = ip
			}
			ctx := context.WithValue(r.Context(), ctxClientIP, strings.TrimSpace(clientIP))
			ctx = context.WithValue(ctx, ctxAccessKeyID, accessKeyID)
			ctx = context.WithValue(ctx, ctxAccessKeyAlias, accessKeyAlias)
			base.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	return Handlers{
		Streamable: authWrap(streamable),
		SSE:        authWrap(sse),
	}
}

func addProxyTool(server *mcp.Server, proxy *services.TavilyProxy, tool *mcp.Tool, method, path string) {
	server.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var body []byte
		if method == http.MethodPost {
			if len(req.Params.Arguments) > 0 {
				body = req.Params.Arguments
			} else {
				body = []byte("{}")
			}
		}

		headers := make(http.Header)
		headers.Set("User-Agent", "tavily-proxy-mcp")
		if method == http.MethodPost {
			headers.Set("Content-Type", "application/json")
		}

		clientIP, _ := ctx.Value(ctxClientIP).(string)
		if clientIP == "" {
			clientIP = "mcp"
		}
		accessKeyID, _ := ctx.Value(ctxAccessKeyID).(uint)
		accessKeyAlias, _ := ctx.Value(ctxAccessKeyAlias).(string)

		resp, err := proxy.Do(ctx, services.ProxyRequest{
			Method:         method,
			Path:           path,
			Headers:        headers,
			Body:           body,
			ClientIP:       clientIP,
			ContentType:    "application/json",
			AccessKeyID:    accessKeyID,
			AccessKeyAlias: accessKeyAlias,
		})
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
				StructuredContent: map[string]any{"error": err.Error()},
			}, nil
		}

		text := string(resp.Body)

		var parsed any
		if err := json.Unmarshal(resp.Body, &parsed); err != nil {
			parsed = nil
		}

		var structured any
		if m, ok := parsed.(map[string]any); ok {
			structured = m
		} else {
			structured = map[string]any{"raw": text}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Upstream status %d: %s", resp.StatusCode, text)},
				},
				StructuredContent: structured,
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
			StructuredContent: structured,
		}, nil
	})
}

func addResearchGetTool(server *mcp.Server, proxy *services.TavilyProxy, tool *mcp.Tool) {
	server.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			RequestID string `json:"request_id"`
		}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{&mcp.TextContent{Text: "invalid arguments: " + err.Error()}},
					StructuredContent: map[string]any{"error": "invalid arguments"},
				}, nil
			}
		}
		requestID := strings.TrimSpace(args.RequestID)
		if requestID == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "request_id is required"}},
				StructuredContent: map[string]any{"error": "request_id is required"},
			}, nil
		}

		headers := make(http.Header)
		headers.Set("User-Agent", "tavily-proxy-mcp")
		headers.Set("Accept", "application/json")

		clientIP, _ := ctx.Value(ctxClientIP).(string)
		if clientIP == "" {
			clientIP = "mcp"
		}
		accessKeyID, _ := ctx.Value(ctxAccessKeyID).(uint)
		accessKeyAlias, _ := ctx.Value(ctxAccessKeyAlias).(string)

		resp, err := proxy.Do(ctx, services.ProxyRequest{
			Method:         http.MethodGet,
			Path:           "/research/" + requestID,
			Headers:        headers,
			ClientIP:       clientIP,
			AccessKeyID:    accessKeyID,
			AccessKeyAlias: accessKeyAlias,
		})
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
				StructuredContent: map[string]any{"error": err.Error()},
			}, nil
		}

		text := string(resp.Body)
		var parsed any
		if err := json.Unmarshal(resp.Body, &parsed); err != nil {
			parsed = nil
		}
		var structured any
		if m, ok := parsed.(map[string]any); ok {
			structured = m
		} else {
			structured = map[string]any{"raw": text}
		}

		// 200 completed/failed, 202 pending/in_progress 均返回给调用方
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Upstream status %d: %s", resp.StatusCode, text)},
				},
				StructuredContent: structured,
			}, nil
		}

		return &mcp.CallToolResult{
			Content:            []mcp.Content{&mcp.TextContent{Text: text}},
			StructuredContent:  structured,
		}, nil
	})
}

func addUsageTool(server *mcp.Server, stats *services.StatsService, tool *mcp.Tool) {
	server.AddTool(tool, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if stats == nil {
			const msg = "stats service unavailable"
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: msg},
				},
				StructuredContent: map[string]any{"error": msg},
			}, nil
		}

		s, err := stats.Get(ctx)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
				StructuredContent: map[string]any{"error": err.Error()},
			}, nil
		}

		payload := map[string]any{
			"key": map[string]any{
				"usage": s.TotalUsed,
				"limit": s.TotalQuota,
			},
		}

		raw, err := json.Marshal(payload)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: err.Error()},
				},
				StructuredContent: map[string]any{"error": err.Error()},
			}, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(raw)},
			},
			StructuredContent: payload,
		}, nil
	})
}

var tavilySearchInputSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": true,
	"required":             []string{"query"},
	"properties": map[string]any{
		"query": map[string]any{
			"type":        "string",
			"description": "The search query to execute with Tavily.",
		},
		"auto_parameters": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "Automatically configures search parameters based on the query. Explicit values override auto-selected ones. Note: include_answer/include_raw_content/max_results must be set manually. auto_parameters may set search_depth=advanced (2 credits); set search_depth=basic to avoid extra cost.",
		},
		"topic": map[string]any{
			"type":        "string",
			"enum":        []string{"general", "news", "finance"},
			"default":     "general",
			"description": "Search topic/category. Use news for real-time updates; general for broad searches.",
		},
		"search_depth": map[string]any{
			"type":        "string",
			"enum":        []string{"advanced", "basic", "fast", "ultra-fast"},
			"default":     "basic",
			"description": "Controls relevance vs latency and how results[].content is generated. basic: balanced, 1 summary per URL (1 credit). fast: lower latency, multiple snippets per URL (1 credit). ultra-fast: lowest latency, 1 summary per URL (1 credit). advanced: highest relevance, multiple snippets per URL (2 credits).",
		},
		"chunks_per_source": map[string]any{
			"type":        "integer",
			"minimum":     1,
			"maximum":     3,
			"default":     3,
			"description": "Max number of relevant chunks (each up to ~500 chars) to return per source. Used with search_depth=advanced.",
		},
		"max_results": map[string]any{
			"type":        "integer",
			"minimum":     0,
			"maximum":     20,
			"default":     5,
			"description": "The maximum number of search results to return.",
		},
		"time_range": map[string]any{
			"type":        "string",
			"enum":        []string{"day", "week", "month", "year", "d", "w", "m", "y"},
			"default":     nil,
			"description": "Filter results by publish/updated time window back from now (day/week/month/year or d/w/m/y).",
		},
		"start_date": map[string]any{
			"type":        "string",
			"format":      "date",
			"default":     nil,
			"description": "Return results after this date (YYYY-MM-DD).",
		},
		"end_date": map[string]any{
			"type":        "string",
			"format":      "date",
			"default":     nil,
			"description": "Return results before this date (YYYY-MM-DD).",
		},
		"country": map[string]any{
			"type":        "string",
			"default":     nil,
			"description": "Boost results from a specific country (topic=general only). Use lowercase country names like 'united states'.",
		},
		"include_domains": map[string]any{
			"type":        "array",
			"default":     []any{},
			"items":       map[string]any{"type": "string"},
			"description": "A list of domains to specifically include in the search results (max 300).",
		},
		"exclude_domains": map[string]any{
			"type":        "array",
			"default":     []any{},
			"items":       map[string]any{"type": "string"},
			"description": "A list of domains to specifically exclude from the search results (max 150).",
		},
		"include_images": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "Also perform an image search and include images in the response.",
		},
		"include_image_descriptions": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "When include_images is true, also add a descriptive text for each image.",
		},
		"include_answer": map[string]any{
			"description": "Include an LLM-generated answer to the query. true/basic: quick answer; advanced: more detailed.",
			"oneOf": []any{
				map[string]any{"type": "boolean"},
				map[string]any{"type": "string", "enum": []string{"basic", "advanced"}},
			},
			"default": false,
		},
		"include_raw_content": map[string]any{
			"description": "Include cleaned/parsed page content for each result. true/markdown: markdown; text: plain text (may increase latency).",
			"oneOf": []any{
				map[string]any{"type": "boolean"},
				map[string]any{"type": "string", "enum": []string{"markdown", "text"}},
			},
			"default": false,
		},
		"include_favicon": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "Whether to include the favicon URL for each result.",
		},
		"include_usage": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "Whether to include credit usage information in the response.",
		},
		"exact_match": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "Ensure that only search results containing the exact quoted phrase(s) in the query are returned, bypassing synonyms or semantic variations.",
		},
	},
}

var tavilyExtractInputSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": true,
	"required":             []string{"urls"},
	"properties": map[string]any{
		"urls": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "URLs to extract content from.",
		},
		"query": map[string]any{
			"type":        "string",
			"description": "User intent for reranking extracted content chunks.",
		},
		"chunks_per_source": map[string]any{
			"type":        "integer",
			"minimum":     1,
			"maximum":     5,
			"default":     3,
			"description": "Max content chunks (up to ~500 chars each) to return per source. Only available when query is provided.",
		},
		"extract_depth": map[string]any{
			"type":        "string",
			"enum":        []string{"basic", "advanced"},
			"default":     "basic",
			"description": "Depth of extraction.",
		},
		"format": map[string]any{
			"type":        "string",
			"enum":        []string{"markdown", "text"},
			"default":     "markdown",
			"description": "Output format for extracted content.",
		},
		"include_images": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "Include images.",
		},
		"include_favicon": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "Include favicon URL.",
		},
		"timeout": map[string]any{
			"type":        "number",
			"minimum":     1,
			"maximum":     60,
			"description": "Max seconds to wait per URL extraction before timing out. Defaults: 10s basic, 30s advanced.",
		},
		"include_usage": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "Include credit usage information in the response.",
		},
	},
}

var tavilyMapInputSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": true,
	"required":             []string{"url"},
	"properties": map[string]any{
		"url": map[string]any{
			"type":        "string",
			"description": "Root URL to begin mapping.",
		},
		"instructions": map[string]any{
			"type":        "string",
			"description": "Natural language instructions for the crawler.",
		},
		"max_depth": map[string]any{
			"type":        "integer",
			"minimum":     1,
			"maximum":     5,
			"default":     1,
			"description": "Max depth of mapping.",
		},
		"max_breadth": map[string]any{
			"type":        "integer",
			"minimum":     1,
			"maximum":     500,
			"default":     20,
			"description": "Max number of links to follow per level.",
		},
		"limit": map[string]any{
			"type":        "integer",
			"minimum":     1,
			"default":     50,
			"description": "Total number of links to process.",
		},
		"select_paths": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Regex patterns to include specific paths.",
		},
		"select_domains": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Regex patterns to include specific domains.",
		},
		"exclude_paths": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Regex patterns to exclude paths.",
		},
		"exclude_domains": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Regex patterns to exclude domains.",
		},
		"allow_external": map[string]any{
			"type":        "boolean",
			"default":     true,
			"description": "Allow following external-domain links.",
		},
		"timeout": map[string]any{
			"type":        "number",
			"minimum":     10,
			"maximum":     150,
			"default":     150,
			"description": "Maximum time in seconds to wait for the map operation before timing out.",
		},
		"include_usage": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "Include credit usage information in the response.",
		},
	},
}

var tavilyCrawlInputSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": true,
	"required":             []string{"url"},
	"properties": map[string]any{
		"url": map[string]any{
			"type":        "string",
			"description": "Root URL to begin crawling.",
		},
		"instructions": map[string]any{
			"type":        "string",
			"description": "Natural language instructions for the crawler.",
		},
		"max_depth": map[string]any{
			"type":        "integer",
			"minimum":     1,
			"maximum":     5,
			"default":     1,
			"description": "Max depth of crawl.",
		},
		"max_breadth": map[string]any{
			"type":        "integer",
			"minimum":     1,
			"maximum":     500,
			"default":     20,
			"description": "Max number of links to follow per level.",
		},
		"limit": map[string]any{
			"type":        "integer",
			"minimum":     1,
			"default":     50,
			"description": "Total number of pages to process.",
		},
		"select_paths": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Regex patterns to include specific paths.",
		},
		"select_domains": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Regex patterns to include specific domains.",
		},
		"exclude_paths": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Regex patterns to exclude paths.",
		},
		"exclude_domains": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Regex patterns to exclude domains.",
		},
		"allow_external": map[string]any{
			"type":        "boolean",
			"default":     true,
			"description": "Allow following external-domain links.",
		},
		"chunks_per_source": map[string]any{
			"type":        "integer",
			"minimum":     1,
			"maximum":     5,
			"default":     3,
			"description": "Number of content chunks (max ~500 chars each) to return per source. Only available when instructions are provided.",
		},
		"include_images": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "Include images discovered during crawling.",
		},
		"extract_depth": map[string]any{
			"type":        "string",
			"enum":        []string{"basic", "advanced"},
			"default":     "basic",
			"description": "Extraction depth for crawled pages.",
		},
		"format": map[string]any{
			"type":        "string",
			"enum":        []string{"markdown", "text"},
			"default":     "markdown",
			"description": "Format of extracted content.",
		},
		"include_favicon": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "Include favicon URL for each result.",
		},
		"timeout": map[string]any{
			"type":        "number",
			"minimum":     10,
			"maximum":     150,
			"default":     150,
			"description": "Maximum time in seconds to wait for the crawl operation before timing out.",
		},
		"include_usage": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "Include credit usage information in the response.",
		},
	},
}

var tavilyResearchInputSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": true,
	"required":             []string{"input"},
	"properties": map[string]any{
		"input": map[string]any{
			"type":        "string",
			"description": "The research task or question to investigate.",
		},
		"model": map[string]any{
			"type":        "string",
			"enum":        []string{"mini", "pro", "auto"},
			"default":     "auto",
			"description": "Research agent model. mini: targeted/efficient (min 4 credits); pro: comprehensive (min 15 credits); auto: let Tavily choose.",
		},
		"stream": map[string]any{
			"type":        "boolean",
			"default":     false,
			"description": "SSE streaming. Keep false when using this MCP tool; streaming is not supported through the proxy tool path.",
		},
		"output_schema": map[string]any{
			"type":        "object",
			"description": "JSON Schema for structured research output. Must include properties; optional required.",
		},
		"citation_format": map[string]any{
			"type":        "string",
			"enum":        []string{"numbered", "mla", "apa", "chicago"},
			"default":     "numbered",
			"description": "Citation format in the research report.",
		},
		"include_domains": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Soft preference domains (max 20). Prioritized but not exclusive.",
		},
		"exclude_domains": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Hard blocklist domains and subdomains (max 20).",
		},
		"output_length": map[string]any{
			"type":        "string",
			"enum":        []string{"short", "standard", "long"},
			"default":     "standard",
			"description": "Target response size (soft guidance).",
		},
		"files": map[string]any{
			"type":        "array",
			"description": "Optional files (.txt/.md/.json) as base64 sources. Max 5 files.",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
					"data": map[string]any{"type": "string"},
					"type": map[string]any{"type": "string", "enum": []string{"base64"}},
				},
			},
		},
	},
}

var tavilyResearchGetInputSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"request_id"},
	"properties": map[string]any{
		"request_id": map[string]any{
			"type":        "string",
			"description": "Research task request_id returned by tavily-research / POST /research.",
		},
	},
}

func parseBearerToken(authHeader string) string {
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
