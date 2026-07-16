package services

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"tavily-proxy/server/internal/models"
)

type QuotaSyncService struct {
	keys   *KeyService
	proxy  *TavilyProxy
	logger *slog.Logger
}

type QuotaSyncItemResult struct {
	ID         uint   `json:"id"`
	Alias      string `json:"alias"`
	Status     string `json:"status"` // ok|error
	Error      string `json:"error,omitempty"`
	UsedQuota  int    `json:"used_quota,omitempty"`
	TotalQuota int    `json:"total_quota,omitempty"`
}

type QuotaSyncResult struct {
	Total     int                   `json:"total"`
	Succeeded int                   `json:"succeeded"`
	Failed    int                   `json:"failed"`
	Items     []QuotaSyncItemResult `json:"items"`
	StartedAt time.Time             `json:"started_at"`
	EndedAt   time.Time             `json:"ended_at"`
}

const defaultQuotaSyncConcurrency = 4
const maxQuotaSyncConcurrency = 32
const maxQuotaSyncInterval = 60 * time.Second

func NewQuotaSyncService(keys *KeyService, proxy *TavilyProxy, logger *slog.Logger) *QuotaSyncService {
	return &QuotaSyncService{keys: keys, proxy: proxy, logger: logger}
}

func (s *QuotaSyncService) SyncOne(ctx context.Context, id uint) (QuotaSyncItemResult, error) {
	key, err := s.keys.Get(ctx, id)
	if err != nil {
		return QuotaSyncItemResult{}, err
	}
	item := s.syncKey(ctx, *key)
	if item.Status != "ok" {
		return item, errors.New(item.Error)
	}
	return item, nil
}

func (s *QuotaSyncService) SyncAll(ctx context.Context) (QuotaSyncResult, error) {
	return s.SyncAllWithConcurrencyAndInterval(ctx, defaultQuotaSyncConcurrency, 0)
}

func (s *QuotaSyncService) SyncAllWithConcurrency(ctx context.Context, concurrency int) (QuotaSyncResult, error) {
	return s.SyncAllWithConcurrencyAndInterval(ctx, concurrency, 0)
}

func (s *QuotaSyncService) SyncAllWithConcurrencyAndInterval(ctx context.Context, concurrency int, interval time.Duration) (QuotaSyncResult, error) {
	started := time.Now()
	keyItems, err := s.keys.List(ctx)
	if err != nil {
		return QuotaSyncResult{}, err
	}

	results := make([]QuotaSyncItemResult, len(keyItems))
	var succeeded, failed int

	if len(keyItems) == 0 {
		return QuotaSyncResult{
			Total:     0,
			Succeeded: 0,
			Failed:    0,
			Items:     results,
			StartedAt: started,
			EndedAt:   time.Now(),
		}, nil
	}

	if concurrency <= 0 {
		concurrency = defaultQuotaSyncConcurrency
	}
	if concurrency > maxQuotaSyncConcurrency {
		concurrency = maxQuotaSyncConcurrency
	}
	if concurrency > len(keyItems) {
		concurrency = len(keyItems)
	}

	if interval < 0 {
		interval = 0
	}
	if interval > maxQuotaSyncInterval {
		interval = maxQuotaSyncInterval
	}

	var paceMu sync.Mutex
	var nextStart time.Time
	waitForSlot := func() error {
		if interval <= 0 {
			return nil
		}

		var sleep time.Duration
		paceMu.Lock()
		now := time.Now()
		if nextStart.IsZero() || now.After(nextStart) {
			nextStart = now.Add(interval)
			paceMu.Unlock()
			return nil
		}
		sleep = nextStart.Sub(now)
		nextStart = nextStart.Add(interval)
		paceMu.Unlock()

		timer := time.NewTimer(sleep)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}

	type job struct {
		idx int
		key models.APIKey
	}
	jobs := make(chan job)

	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := range jobs {
				if err := waitForSlot(); err != nil {
					item := QuotaSyncItemResult{
						ID:     j.key.ID,
						Alias:  j.key.Alias,
						Status: "error",
						Error:  err.Error(),
					}
					mu.Lock()
					failed++
					results[j.idx] = item
					mu.Unlock()
					continue
				}

				item := s.syncKey(ctx, j.key)

				mu.Lock()
				if item.Status == "ok" {
					succeeded++
				} else {
					failed++
				}
				results[j.idx] = item
				mu.Unlock()
			}
		}()
	}

	for i, k := range keyItems {
		jobs <- job{idx: i, key: k}
	}
	close(jobs)

	wg.Wait()

	return QuotaSyncResult{
		Total:     len(keyItems),
		Succeeded: succeeded,
		Failed:    failed,
		Items:     results,
		StartedAt: started,
		EndedAt:   time.Now(),
	}, nil
}

func (s *QuotaSyncService) syncKey(ctx context.Context, key models.APIKey) QuotaSyncItemResult {
	item := QuotaSyncItemResult{ID: key.ID, Alias: key.Alias}
	usage, limit, err := s.proxy.GetUsage(ctx, key.Key)
	if err != nil {
		item.Status = "error"
		item.Error = err.Error()
		if ue := (*UpstreamStatusError)(nil); errors.As(err, &ue) {
			switch ue.StatusCode {
			case http.StatusUnauthorized:
				if e := s.keys.MarkInvalid(context.Background(), key.ID); e != nil {
					s.logger.Warn("quota-sync: mark invalid failed", "key_id", key.ID, "err", e)
				}
			case http.StatusForbidden, 432, 433:
				if e := s.keys.MarkExhausted(context.Background(), key.ID); e != nil {
					s.logger.Warn("quota-sync: mark exhausted failed", "key_id", key.ID, "err", e)
				}
			}
		}
		return item
	}

	totalQuota := key.TotalQuota
	if limit != nil && *limit > 0 {
		totalQuota = *limit
	}
	if totalQuota > 0 && usage > totalQuota {
		usage = totalQuota
	}
	// 成功拿到官方 /usage 时以官方值为准，允许覆盖本地更高 used_quota
	// （本地估算可能偏高；月度重置/官方校准依赖此路径）。
	// 上游失败时已在上方 early return，本地额度保持不变。

	// 用 background 而非传入的 ctx：上游用量已消耗，即使请求被取消也必须落库
	if e := s.keys.SetUsage(context.Background(), key.ID, usage, &totalQuota); e != nil {
		s.logger.Warn("quota-sync: persist usage failed", "key_id", key.ID, "err", e)
	}

	item.Status = "ok"
	item.UsedQuota = usage
	item.TotalQuota = totalQuota
	return item
}
