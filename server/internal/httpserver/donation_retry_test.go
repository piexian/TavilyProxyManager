package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"tavily-proxy/server/internal/models"
	"tavily-proxy/server/internal/services"
	"testing"
	"time"
)

func TestDonationCanRetryAfterUsageFailure(t *testing.T) {
	for _, portal := range []bool{false, true} {
		name := "public"
		if portal {
			name = "portal"
		}
		t.Run(name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			database := portalTestDB(t)
			keys := services.NewKeyService(database, logger)
			access := services.NewAccessKeyService(database, logger)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(503)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"key":{"usage":0,"limit":1000}}`)
			}))
			defer upstream.Close()
			proxy := services.NewTavilyProxy(upstream.URL, time.Second, keys, nil, nil, logger)
			for attempt := 0; attempt < 2; attempt++ {
				c, w := portalCtx("POST", "/donate", `{"keys":["tvly-abcdefghijklmnop"],"alias":"retry-user"}`, 1, "retry-user")
				if portal {
					handlePortalAddKeys(c, keys, proxy)
				} else {
					handleDonateKeys(c, keys, proxy, access)
				}
				if w.Code != 200 {
					t.Fatalf("attempt %d status=%d body=%s", attempt, w.Code, w.Body.String())
				}
				record, err := keys.QueryByKey(context.Background(), "tvly-abcdefghijklmnop")
				if err != nil {
					t.Fatal(err)
				}
				if attempt == 0 && record != nil {
					t.Fatal("failed validation persisted a key")
				}
				if attempt == 1 && (record == nil || !record.IsActive || record.IsInvalid || record.AccessKeyID == 0) {
					t.Fatalf("retry did not activate and bind key: %+v", record)
				}
			}
		})
	}
}

func TestUnownedHistoricalDonationReservesAlias(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	keys := services.NewKeyService(database, logger)
	access := services.NewAccessKeyService(database, logger)
	legacy := models.APIKey{Key: "tvly-legacy", Alias: " Historical ", IsDonated: true, AccessKeyID: 0}
	if err := database.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	c, w := portalCtx("POST", "/donate", `{"keys":["tvly-abcdefghijklmnop"],"alias":"historical"}`, 0, "")
	handleDonateKeys(c, keys, nil, access)
	if w.Code != 409 {
		t.Fatalf("alias was not reserved: status=%d body=%s", w.Code, w.Body.String())
	}
	if err := database.First(&legacy, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if legacy.AccessKeyID != 0 {
		t.Fatal("historical ownership changed")
	}
}
