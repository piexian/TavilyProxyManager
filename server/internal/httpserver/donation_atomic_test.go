package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"tavily-proxy/server/internal/services"
)

const (
	donateKeyA = "tvly-aaaaaaaaaaaaaaaa"
	donateKeyB = "tvly-bbbbbbbbbbbbbbbb"
)

func publicDonateCtx(ctx context.Context, body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/public/donate", strings.NewReader(body)).WithContext(ctx)
	return c, w
}

// 捐赠入库的 Key 必须一步到位：已激活、已归属于返回的访问密钥、带上游额度；
// 同一请求里重复的 Key 只入库一次。
func TestPublicDonation_KeysAreActiveOwnedAndCarryUsage(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	keys := services.NewKeyService(database, logger)
	access := services.NewAccessKeyService(database, logger)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"key":{"usage":5,"limit":200}}`)
	}))
	t.Cleanup(upstream.Close)
	proxy := services.NewTavilyProxy(upstream.URL, time.Second, keys, nil, nil, logger)

	c, w := publicDonateCtx(context.Background(),
		`{"keys":["`+donateKeyA+`","`+donateKeyB+`","`+donateKeyA+`"],"alias":"owner"}`)
	handleDonateKeys(c, keys, proxy, access)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Created    int    `json:"created"`
		Duplicated int    `json:"duplicated"`
		AccessKey  string `json:"access_key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Created != 2 || resp.Duplicated != 1 || resp.AccessKey == "" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	list, err := access.List(context.Background())
	if err != nil || len(list) != 1 || list[0].Key != resp.AccessKey {
		t.Fatalf("access keys: %+v err=%v", list, err)
	}
	for _, key := range []string{donateKeyA, donateKeyB} {
		record, err := keys.QueryByKey(context.Background(), key)
		if err != nil || record == nil {
			t.Fatalf("key %s not persisted: %v", key, err)
		}
		if !record.IsActive || !record.IsDonated || record.AccessKeyID != list[0].ID {
			t.Fatalf("key %s not active+owned: %+v", key, record)
		}
		if record.UsedQuota != 5 || record.TotalQuota != 200 {
			t.Fatalf("key %s quota used=%d total=%d, want 5/200", key, record.UsedQuota, record.TotalQuota)
		}
	}
}

// 校验阶段客户端断连：已通过校验的 Key 仍要连同访问密钥完整入库（有主且激活），
// 未通过的不入库；不能留下「已激活但无主」的 Key。
func TestPublicDonation_CancelDuringValidationLeavesNoOwnerlessKey(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	keys := services.NewKeyService(database, logger)
	access := services.NewAccessKeyService(database, logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 2 {
			cancel() // 校验第二个 Key 时客户端断开
			time.Sleep(100 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"key":{"usage":0,"limit":1000}}`)
	}))
	t.Cleanup(upstream.Close)
	proxy := services.NewTavilyProxy(upstream.URL, 5*time.Second, keys, nil, nil, logger)

	c, w := publicDonateCtx(ctx, `{"keys":["`+donateKeyA+`","`+donateKeyB+`"],"alias":"owner"}`)
	handleDonateKeys(c, keys, proxy, access)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	list, err := access.List(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("access keys: %+v err=%v", list, err)
	}
	a, _ := keys.QueryByKey(context.Background(), donateKeyA)
	if a == nil || !a.IsActive || a.AccessKeyID != list[0].ID {
		t.Fatalf("validated key must be persisted active+owned: %+v", a)
	}
	if b, _ := keys.QueryByKey(context.Background(), donateKeyB); b != nil {
		t.Fatalf("unvalidated key must not be persisted: %+v", b)
	}
}

// 没有任何 Key 通过校验时不创建访问密钥，alias 不会被白白占住。
func TestPublicDonation_AllInvalidKeepsAliasFree(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	keys := services.NewKeyService(database, logger)
	access := services.NewAccessKeyService(database, logger)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(upstream.Close)
	proxy := services.NewTavilyProxy(upstream.URL, time.Second, keys, nil, nil, logger)

	c, w := publicDonateCtx(context.Background(), `{"keys":["`+donateKeyA+`"],"alias":"owner"}`)
	handleDonateKeys(c, keys, proxy, access)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "access_key") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if taken, err := access.AliasExists(context.Background(), "owner", 0); err != nil || taken {
		t.Fatalf("alias must stay free: taken=%v err=%v", taken, err)
	}
}

// 管理端编辑弹窗每次都会带上原 alias：历史重名时，不改 alias 的更新不能被 409 卡死；
// 真正改成他人占用的 alias 仍须拒绝。
func TestHandleUpdateAccessKey_UnchangedAliasSkipsUniquenessCheck(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	access := services.NewAccessKeyService(database, logger)
	ctx := context.Background()
	first, err := access.Create(ctx, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := access.Create(ctx, "Default"); err != nil { // 历史遗留重名
		t.Fatal(err)
	}
	if _, err := access.Create(ctx, "Taken"); err != nil {
		t.Fatal(err)
	}

	update := func(body string) int {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/access-keys/x", strings.NewReader(body))
		handleUpdateAccessKey(c, access, strconv.Itoa(int(first.ID)))
		return w.Code
	}

	if code := update(`{"alias":"Default","is_active":false}`); code != http.StatusOK {
		t.Fatalf("unchanged alias: got %d want 200", code)
	}
	if got, _ := access.FindByID(ctx, first.ID); got == nil || got.IsActive {
		t.Fatalf("is_active not updated: %+v", got)
	}
	if code := update(`{"alias":"taken"}`); code != http.StatusConflict {
		t.Fatalf("changed to taken alias: got %d want 409", code)
	}
}

// 管理员可显式把无主 Key 指派给访问密钥（认领升级前的捐赠），目标不存在则拒绝。
func TestHandleUpdateKey_AssignAccessKey(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database := portalTestDB(t)
	keys := services.NewKeyService(database, logger)
	access := services.NewAccessKeyService(database, logger)
	ctx := context.Background()
	ak, err := access.Create(ctx, "donor")
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := keys.Create(ctx, "tvly-orphan-key-0000", "legacy", 1000)
	if err != nil {
		t.Fatal(err)
	}
	deps := Dependencies{KeyService: keys, AccessKeyService: access}

	put := func(body string) int {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/keys/x", strings.NewReader(body))
		handleUpdateKey(c, deps, strconv.Itoa(int(orphan.ID)))
		return w.Code
	}
	owner := func() uint {
		got, err := keys.Get(ctx, orphan.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.AccessKeyID
	}

	if code := put(`{"access_key_id":9999}`); code != http.StatusBadRequest || owner() != 0 {
		t.Fatalf("unknown access key: code=%d owner=%d", code, owner())
	}
	if code := put(`{"access_key_id":` + strconv.Itoa(int(ak.ID)) + `}`); code != http.StatusOK || owner() != ak.ID {
		t.Fatalf("assign: code=%d owner=%d want %d", code, owner(), ak.ID)
	}
	if code := put(`{"access_key_id":0}`); code != http.StatusOK || owner() != 0 {
		t.Fatalf("unassign: code=%d owner=%d", code, owner())
	}
}
