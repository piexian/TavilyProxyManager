package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"tavily-proxy/server/internal/db"
	"tavily-proxy/server/internal/models"
	"tavily-proxy/server/internal/services"
)

func portalTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return database
}

func portalCtx(method, target string, body string, accessKeyID uint, accessKeyAlias string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	c.Request = httptest.NewRequest(method, target, reader)
	c.Set(ctxAccessKeyID, accessKeyID)
	c.Set(ctxAccessKeyAlias, accessKeyAlias)
	return c, w
}

// 删除非己 Key 必须 403，且不真正删除；删除自己的 Key 正常 200。
func TestHandlePortalDeleteKey_OwnershipEnforced(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	keys := services.NewKeyService(database, logger)
	ctx := context.Background()

	owned := models.APIKey{Key: "tvly-owned", Alias: "owner", AccessKeyID: 1, IsActive: true, IsDonated: true}
	if err := database.WithContext(ctx).Create(&owned).Error; err != nil {
		t.Fatalf("create owned key: %v", err)
	}

	// 另一个捐赠者(ID=2)尝试删除 ID=1 名下的 Key → 403
	c, w := portalCtx(http.MethodDelete, "/api/portal/keys/"+strconv.Itoa(int(owned.ID)), "", 2, "other")
	handlePortalDeleteKey(c, keys, strconv.Itoa(int(owned.ID)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-owner delete: got %d want 403 (body=%q)", w.Code, w.Body.String())
	}
	if got, _ := keys.FindByID(ctx, owned.ID); got == nil {
		t.Fatalf("key should still exist after forbidden delete")
	}

	// 归属者(ID=1)删除自己的 Key → 200
	c2, w2 := portalCtx(http.MethodDelete, "/api/portal/keys/"+strconv.Itoa(int(owned.ID)), "", 1, "owner")
	handlePortalDeleteKey(c2, keys, strconv.Itoa(int(owned.ID)))
	if w2.Code != http.StatusOK {
		t.Fatalf("owner delete: got %d want 200 (body=%q)", w2.Code, w2.Body.String())
	}
	if got, _ := keys.FindByID(ctx, owned.ID); got != nil {
		t.Fatalf("key should be deleted by owner")
	}
}

// 重置访问密钥后旧 token 立即失效、新 token 生效。
func TestHandlePortalResetKey_InvalidatesOldToken(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	accessKeys := services.NewAccessKeyService(database, logger)
	ctx := context.Background()

	ak, err := accessKeys.Create(ctx, "tester")
	if err != nil {
		t.Fatalf("create access key: %v", err)
	}
	oldToken := ak.Key
	if _, ok := accessKeys.Authenticate(oldToken); !ok {
		t.Fatalf("old token should authenticate before reset")
	}

	c, w := portalCtx(http.MethodPost, "/api/portal/reset-key", "", ak.ID, ak.Alias)
	handlePortalResetKey(c, accessKeys)
	if w.Code != http.StatusOK {
		t.Fatalf("reset: got %d want 200 (body=%q)", w.Code, w.Body.String())
	}
	var out struct {
		AccessKey string `json:"access_key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.AccessKey == "" || out.AccessKey == oldToken {
		t.Fatalf("expected a new non-empty token, got %q (old=%q)", out.AccessKey, oldToken)
	}
	if _, ok := accessKeys.Authenticate(oldToken); ok {
		t.Fatalf("old token must be rejected after reset")
	}
	if _, ok := accessKeys.Authenticate(out.AccessKey); !ok {
		t.Fatalf("new token must authenticate after reset")
	}
}

// 用已存在的 alias 捐赠必须被拒（409 alias_taken），且不触达 Key 处理。
func TestHandleDonateKeys_AliasTaken(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	ctx := context.Background()

	keys := services.NewKeyService(database, logger)
	logs := services.NewLogService(database, logger)
	stats := services.NewStatsService(database)
	proxy := services.NewTavilyProxy("http://127.0.0.1:0", time.Second, keys, logs, stats, logger)
	accessKeys := services.NewAccessKeyService(database, logger)
	if _, err := accessKeys.Create(ctx, "tester"); err != nil {
		t.Fatalf("seed access key: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/public/donate",
		strings.NewReader(`{"keys":["tvly-abcdefghijklmnop"],"alias":"Tester"}`)) // 大小写不同也算重名

	handleDonateKeys(c, keys, proxy, accessKeys)

	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate alias: got %d want 409 (body=%q)", w.Code, w.Body.String())
	}
	var out map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["error"] != "alias_taken" {
		t.Fatalf("expected alias_taken, got %q", out["error"])
	}
}

// 门户时序：系列A=名下 Key 被调用次数，系列B=本人发起请求数。
func TestHandlePortalTimeSeries_TwoSeries(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	ctx := context.Background()
	keys := services.NewKeyService(database, logger)
	stats := services.NewStatsService(database)

	const akID = uint(7)
	k1 := models.APIKey{Key: "tvly-k1", Alias: "owner", AccessKeyID: akID, IsActive: true, IsDonated: true}
	k2 := models.APIKey{Key: "tvly-k2", Alias: "owner", AccessKeyID: akID, IsActive: true, IsDonated: true}
	if err := database.Create(&k1).Error; err != nil {
		t.Fatalf("create k1: %v", err)
	}
	if err := database.Create(&k2).Error; err != nil {
		t.Fatalf("create k2: %v", err)
	}

	logsSvc := services.NewLogService(database, logger)
	// 系列A：5 条由他名下 Key 服务（KeyUsed ∈ {k1,k2}），不归他发起（AccessKeyID=0）
	for i := 0; i < 3; i++ {
		_ = logsSvc.Create(ctx, &models.RequestLog{RequestID: "a", KeyUsed: k1.ID, Endpoint: "/search", StatusCode: 200})
	}
	for i := 0; i < 2; i++ {
		_ = logsSvc.Create(ctx, &models.RequestLog{RequestID: "a", KeyUsed: k2.ID, Endpoint: "/search", StatusCode: 200})
	}
	// 系列B：4 条由他本人发起（AccessKeyID=akID），用的不是他名下 Key
	for i := 0; i < 4; i++ {
		_ = logsSvc.Create(ctx, &models.RequestLog{RequestID: "b", KeyUsed: 999, AccessKeyID: akID, Endpoint: "/search", StatusCode: 200})
	}

	c, w := portalCtx(http.MethodGet, "/api/portal/timeseries?granularity=day", "", akID, "owner")
	c.Request.URL.RawQuery = "granularity=day"
	handlePortalTimeSeries(c, keys, stats)
	if w.Code != http.StatusOK {
		t.Fatalf("timeseries: got %d want 200 (body=%q)", w.Code, w.Body.String())
	}

	var ts services.TimeSeries
	if err := json.Unmarshal(w.Body.Bytes(), &ts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(ts.Series) != 2 {
		t.Fatalf("expected 2 series, got %d", len(ts.Series))
	}
	if ts.Series[0].Name != "Served By My Keys" || ts.Series[1].Name != "My Requests" {
		t.Fatalf("unexpected series names: %q / %q", ts.Series[0].Name, ts.Series[1].Name)
	}
	sum := func(d []int64) int64 {
		var s int64
		for _, v := range d {
			s += v
		}
		return s
	}
	if got := sum(ts.Series[0].Data); got != 5 {
		t.Fatalf("series A (served by my keys) sum: got %d want 5", got)
	}
	if got := sum(ts.Series[1].Data); got != 4 {
		t.Fatalf("series B (my requests) sum: got %d want 4", got)
	}
}

// 禁用登录的访问密钥：门户中间件拒绝(401 login_disabled)，但 Authenticate 仍放行
// （证明代理 NoRoute / MCP 复用同一方法且不检查该字段，API/MCP 调用不受影响）。
func TestAccessKeyAuthMiddleware_LoginDisabled(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	accessKeys := services.NewAccessKeyService(database, logger)
	ctx := context.Background()

	ak, err := accessKeys.Create(ctx, "api-only")
	if err != nil {
		t.Fatalf("create access key: %v", err)
	}
	token := ak.Key
	disabled := true
	if _, err := accessKeys.Update(ctx, ak.ID, services.AccessKeyUpdate{LoginDisabled: &disabled}); err != nil {
		t.Fatalf("disable login: %v", err)
	}

	// 门户中间件：禁用登录 → 401 + login_disabled，并 Abort
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/portal/me", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)
	accessKeyAuthMiddleware(accessKeys)(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("portal middleware: got %d want 401 (body=%q)", w.Code, w.Body.String())
	}
	var out map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["error"] != "login_disabled" {
		t.Fatalf("expected error login_disabled, got %q", out["error"])
	}
	if !c.IsAborted() {
		t.Fatalf("middleware should abort for a login-disabled key")
	}

	// 代理/MCP 复用的 Authenticate 仍放行
	if got, ok := accessKeys.Authenticate(token); !ok || got == nil {
		t.Fatalf("Authenticate must still succeed for API/MCP after login disabled")
	}

	// 对照：允许登录的密钥应顺利通过中间件并写入上下文
	ak2, err := accessKeys.Create(ctx, "can-login")
	if err != nil {
		t.Fatalf("create second access key: %v", err)
	}
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/api/portal/me", nil)
	c2.Request.Header.Set("Authorization", "Bearer "+ak2.Key)
	accessKeyAuthMiddleware(accessKeys)(c2)
	if c2.IsAborted() {
		t.Fatalf("middleware should allow a login-enabled key (code=%d body=%q)", w2.Code, w2.Body.String())
	}
	if got := c2.GetUint(ctxAccessKeyID); got != ak2.ID {
		t.Fatalf("ctx access key id: got %d want %d", got, ak2.ID)
	}
}
