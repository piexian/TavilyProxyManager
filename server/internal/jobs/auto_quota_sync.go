package jobs

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"tavily-proxy/server/internal/services"
)

func StartAutoQuotaSync(ctx context.Context, settings *services.SettingsService, sync *services.QuotaSyncService, logger *slog.Logger) {
	var running atomic.Bool

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if running.Load() {
					continue
				}

				enabled, err := settings.GetBool(ctx, services.SettingAutoSyncEnabled, false)
				if err != nil {
					logger.Error("auto-sync: failed to read enabled setting", "err", err)
					continue
				}
				if !enabled {
					continue
				}

				intervalMinutes, err := settings.GetInt(ctx, services.SettingAutoSyncIntervalMinutes, 60)
				if err != nil {
					logger.Error("auto-sync: failed to read interval setting", "err", err)
					continue
				}
				if intervalMinutes < 1 {
					intervalMinutes = 1
				}

				concurrency := 1

				requestIntervalSeconds, err := settings.GetInt(ctx, services.SettingAutoSyncRequestIntervalSeconds, 0)
				if err != nil {
					logger.Error("auto-sync: failed to read request interval setting", "err", err)
					continue
				}
				if requestIntervalSeconds < 0 {
					requestIntervalSeconds = 0
				}
				if requestIntervalSeconds > 60 {
					requestIntervalSeconds = 60
				}

				interval := time.Duration(intervalMinutes) * time.Minute
				lastRunAt, _ := settings.GetTime(ctx, services.SettingAutoSyncLastRunAt)
				if lastRunAt != nil && time.Since(*lastRunAt) < interval {
					continue
				}

				if !running.CompareAndSwap(false, true) {
					continue
				}

				go func() {
					defer running.Store(false)

					// 先落地 lastRunAt：写失败说明 DB 不可写（如被移走/只读），
					// 此时继续打上游只会空转并触发上游限流，直接跳过本次
					now := time.Now()
					if err := settings.SetTime(context.Background(), services.SettingAutoSyncLastRunAt, now); err != nil {
						logger.Error("auto-sync: skip run, cannot persist last_run_at (database may be read-only)", "err", err)
						return
					}

					result, err := sync.SyncAllWithConcurrencyAndInterval(
						ctx,
						concurrency,
						time.Duration(requestIntervalSeconds)*time.Second,
					)
					if err != nil {
						if e := settings.Set(context.Background(), services.SettingAutoSyncLastError, err.Error()); e != nil {
							logger.Warn("auto-sync: failed to persist last_error", "err", e)
						}
						logger.Error("auto-sync: sync failed", "err", err)
						return
					}

					if e := settings.SetTime(context.Background(), services.SettingAutoSyncLastSuccessAt, time.Now()); e != nil {
						logger.Warn("auto-sync: failed to persist last_success_at", "err", e)
					}
					if e := settings.Set(context.Background(), services.SettingAutoSyncLastError, ""); e != nil {
						logger.Warn("auto-sync: failed to clear last_error", "err", e)
					}

					// 全部失败通常意味着上游限流或网络问题，附带一个样例错误便于排查
					if result.Total > 0 && result.Failed == result.Total {
						logger.Warn("auto-sync: all keys failed", "total", result.Total, "sample_error", firstSyncError(result))
					}

					logger.Info(
						"auto-sync: completed",
						"total",
						result.Total,
						"failed",
						result.Failed,
						"interval_seconds",
						requestIntervalSeconds,
					)
				}()
			}
		}
	}()
}

func firstSyncError(result services.QuotaSyncResult) string {
	for _, item := range result.Items {
		if item.Status != "ok" && item.Error != "" {
			return item.Error
		}
	}
	return ""
}
