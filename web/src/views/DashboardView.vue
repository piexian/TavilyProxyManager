<template>
  <n-space vertical size="large" class="dashboard-view">
    <div class="page-header dashboard-header">
      <div class="header-info">
        <h2 class="page-title">{{ t("dashboard.title") }}</h2>
        <div class="page-caption">
          {{ formatNumber(stats?.total_remaining ?? 0) }}
          <span class="caption-separator">/</span>
          {{ formatNumber(stats?.total_quota ?? 0) }}
        </div>
      </div>
      <n-button
        size="small"
        @click="refreshAll"
        :loading="loadingStats || loadingChart"
        type="primary"
        secondary
      >
        <template #icon>
          <n-icon :component="RefreshOutline" />
        </template>
        {{ t("dashboard.refreshData") }}
      </n-button>
    </div>

    <n-grid cols="2 m:5" responsive="screen" :x-gap="12" :y-gap="12">
      <n-gi v-for="card in metricCards" :key="card.key">
        <n-card size="small" class="metric-card" :class="`metric-card--${card.tone}`">
          <div class="metric-card__shine"></div>
          <div class="metric-card__header">
            <span class="metric-card__label">{{ card.label }}</span>
            <div class="metric-card__icon">
              <n-icon :component="card.icon" />
            </div>
          </div>
          <div class="metric-card__value-row">
            <span class="metric-card__value">{{ card.value }}</span>
            <span v-if="card.supporting" class="metric-card__supporting">
              {{ card.supporting }}
            </span>
          </div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card size="small" class="metric-card metric-card--violet">
          <div class="metric-card__shine"></div>
          <div class="metric-card__header">
            <span class="metric-card__label">{{ t('dashboard.stats.cacheHits') }}</span>
            <div class="metric-card__icon">
              <n-icon :component="ServerOutline" />
            </div>
          </div>
          <div class="metric-card__value-row">
            <span class="metric-card__value">{{ formatNumber(cacheStats?.total_hits ?? 0) }}</span>
          </div>
        </n-card>
      </n-gi>
    </n-grid>

    <div class="dashboard-panels">
      <n-card size="small" class="panel-card usage-card">
        <div class="panel-header">
          <div>
            <div class="panel-title">{{ t("dashboard.resourceUsage.title") }}</div>
            <div class="panel-subtitle">
              {{ t("dashboard.resourceUsage.monthlyQuotaConsumption") }}
            </div>
          </div>
          <div class="usage-badge" :class="usageBadgeClass">
            {{ t("dashboard.resourceUsage.usedPct", { pct: usagePercent }) }}
          </div>
        </div>

        <div class="usage-total">
          {{ formatNumber(stats?.total_used ?? 0) }}
          <span>{{ formatNumber(stats?.total_quota ?? 0) }}</span>
        </div>

        <div class="usage-progress">
          <div class="usage-progress__fill" :class="usageBadgeClass" :style="{ width: `${usagePercent}%` }"></div>
        </div>

        <div class="usage-split">
          <div class="usage-split__item">
            <span class="usage-split__label">{{ t("dashboard.stats.remainingQuota") }}</span>
            <strong class="usage-split__value">
              {{ formatNumber(stats?.total_remaining ?? 0) }}
            </strong>
          </div>
          <div class="usage-split__item">
            <span class="usage-split__label">{{ t("dashboard.stats.totalUsed") }}</span>
            <strong class="usage-split__value">
              {{ formatNumber(stats?.total_used ?? 0) }}
            </strong>
          </div>
        </div>
      </n-card>

      <n-card size="small" class="panel-card chart-card">
        <div class="panel-header chart-card__header">
          <div>
            <div class="panel-title">{{ t("dashboard.requestAnalytics.title") }}</div>
            <div class="chart-summary">
              <div
                v-for="item in chartSummaries"
                :key="item.name"
                class="chart-summary__item"
                :class="`chart-summary__item--${item.tone}`"
              >
                <span class="chart-summary__dot"></span>
                <span class="chart-summary__label">{{ item.name }}</span>
                <strong class="chart-summary__value">{{ item.value }}</strong>
              </div>
            </div>
          </div>
          <div class="chart-controls">
            <button
              type="button"
              class="chart-type-toggle"
              :aria-label="t('portal.chartType.toggle')"
              :title="t('portal.chartType.toggle')"
              @click="toggleChartType"
            >
              <n-icon
                :component="chartType === 'bar' ? BarChartOutline : TrendingUpOutline"
              />
              <span>{{
                chartType === "bar"
                  ? t("portal.chartType.bar")
                  : t("portal.chartType.line")
              }}</span>
            </button>
            <n-tabs
              v-model:value="granularity"
              type="segment"
              size="small"
              class="chart-tabs"
            >
              <n-tab-pane name="hour" :tab="t('dashboard.requestAnalytics.hour')" />
              <n-tab-pane name="day" :tab="t('dashboard.requestAnalytics.day')" />
              <n-tab-pane name="month" :tab="t('dashboard.requestAnalytics.month')" />
            </n-tabs>
          </div>
        </div>

        <RequestChart
          :timeseries="timeseries"
          :chart-type="chartType"
          :granularity="granularity"
        />
      </n-card>
    </div>
  </n-space>
</template>

<script setup lang="ts">
import {
  computed,
  nextTick,
  onMounted,
  ref,
  watch,
} from "vue";
import {
  NButton,
  NCard,
  NGi,
  NGrid,
  NIcon,
  NSpace,
  NTabPane,
  NTabs,
  useMessage,
} from "naive-ui";
import {
  BarChartOutline,
  BatteryFullOutline,
  CloudUploadOutline,
  KeyOutline,
  PulseOutline,
  RefreshOutline,
  ServerOutline,
  TrendingUpOutline,
} from "@vicons/ionicons5";
import { api } from "../api/client";
import RequestChart from "../components/RequestChart.vue";
import type { Stats, TimeSeries } from "../types";
import { locale, t } from "../i18n";

const props = defineProps<{
  refreshNonce?: number;
}>();

const message = useMessage();
const loadingStats = ref(false);
const loadingChart = ref(false);
const stats = ref<Stats | null>(null);
const cacheStats = ref<{ enabled: boolean; entry_count: number; total_hits: number; total_size_bytes: number } | null>(null);
const timeseries = ref<TimeSeries | null>(null);
const granularity = ref<"hour" | "day" | "month">("hour");
const chartType = ref<"bar" | "line">("bar");

function toggleChartType() {
  chartType.value = chartType.value === "bar" ? "line" : "bar";
}

const numberFormatter = computed(
  () => new Intl.NumberFormat(locale.value === "zh-CN" ? "zh-CN" : "en-US"),
);

function formatNumber(value: number): string {
  return numberFormatter.value.format(value);
}

const usagePercent = computed(() => {
  if (!stats.value) return 0;
  const total = stats.value.total_quota || 0;
  const used = stats.value.total_used || 0;
  if (total <= 0) return 0;
  return Math.max(0, Math.min(100, Math.round((used / total) * 100)));
});

const usageBadgeClass = computed(() => {
  if (usagePercent.value > 90) return "usage-badge--critical";
  if (usagePercent.value > 70) return "usage-badge--warm";
  return "usage-badge--healthy";
});

const metricCards = computed(() => [
  {
    key: "remaining",
    label: t("dashboard.stats.remainingQuota"),
    value: formatNumber(stats.value?.total_remaining ?? 0),
    supporting: `/ ${formatNumber(stats.value?.total_quota ?? 0)}`,
    icon: BatteryFullOutline,
    tone: "emerald",
  },
  {
    key: "used",
    label: t("dashboard.stats.totalUsed"),
    value: formatNumber(stats.value?.total_used ?? 0),
    supporting: `${usagePercent.value}%`,
    icon: CloudUploadOutline,
    tone: "violet",
  },
  {
    key: "active",
    label: t("dashboard.stats.activeKeys"),
    value: formatNumber(stats.value?.active_key_count ?? 0),
    supporting: `/ ${formatNumber(stats.value?.key_count ?? 0)}`,
    icon: KeyOutline,
    tone: "amber",
  },
  {
    key: "today",
    label: t("dashboard.stats.todayRequests"),
    value: formatNumber(stats.value?.today_requests ?? 0),
    supporting: t(`dashboard.requestAnalytics.${granularity.value}`),
    icon: PulseOutline,
    tone: "rose",
  },
]);

const chartSummaries = computed(() => {
  if (!timeseries.value) return [];
  return timeseries.value.series.map((series, idx) => ({
    name: localizeSeriesName(series.name),
    value: formatNumber(series.data[series.data.length - 1] ?? 0),
    tone: idx === 0 ? "violet" : "emerald",
  }));
});

async function refreshStats() {
  loadingStats.value = true;
  try {
    const [statsRes, cacheRes] = await Promise.all([
      api.get<Stats>("/api/stats"),
      api.get<{ enabled: boolean; entry_count: number; total_hits: number; total_size_bytes: number }>("/api/cache/stats"),
    ]);
    stats.value = statsRes.data;
    cacheStats.value = cacheRes.data;
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("dashboard.errors.loadStats"));
  } finally {
    loadingStats.value = false;
  }
}

async function refreshTimeSeries() {
  loadingChart.value = true;
  try {
    const { data } = await api.get<TimeSeries>("/api/stats/timeseries", {
      params: { granularity: granularity.value },
    });
    timeseries.value = data;
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("dashboard.errors.loadChart"));
  } finally {
    loadingChart.value = false;
  }
}

function localizeSeriesName(name: string): string {
  if (name === "All Requests") return t("dashboard.timeseries.allRequests");
  if (name === "Search") return t("dashboard.timeseries.search");
  return name;
}

async function refreshAll() {
  await refreshStats();
  await refreshTimeSeries();
}

watch(
  () => props.refreshNonce,
  async (value, previous) => {
    if (value === undefined || previous === undefined) return;
    if (value === previous) return;
    await nextTick();
    await refreshAll();
  },
);

watch(granularity, async () => {
  await refreshTimeSeries();
});

onMounted(async () => {
  await refreshStats();
  await refreshTimeSeries();
});
</script>

<style scoped>
.dashboard-view {
  gap: 0;
}

.dashboard-header {
  margin-bottom: 2px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 16px;
}

.header-info {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.page-title {
  margin: 0;
  font-size: 24px;
  font-weight: 800;
  letter-spacing: -0.03em;
}

.page-caption {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: var(--n-text-color-3);
  font-family: var(--n-font-family-mono);
}

.caption-separator {
  opacity: 0.45;
}

.metric-card {
  position: relative;
  overflow: hidden;
  border: var(--comic-border) solid var(--comic-ink);
  box-shadow: var(--comic-shadow);
  transition: transform 0.18s ease, box-shadow 0.18s ease;
}

.metric-card:hover {
  transform: translate(-2px, -2px);
  box-shadow: var(--comic-shadow-lg);
}

.metric-card__shine {
  display: none;
}

.metric-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 22px;
}

.metric-card__label {
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--n-text-color-3);
}

.metric-card__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  font-size: 18px;
  color: #fff;
  border: 2px solid var(--comic-ink);
}

.metric-card__value-row {
  display: flex;
  align-items: flex-end;
  gap: 10px;
}

.metric-card__value {
  position: relative;
  z-index: 1;
  font-size: 34px;
  line-height: 1;
  font-weight: 800;
  letter-spacing: -0.04em;
  font-family: var(--comic-font-display);
}

.metric-card__supporting {
  position: relative;
  z-index: 1;
  margin-bottom: 4px;
  font-size: 13px;
  color: var(--n-text-color-3);
  font-family: var(--n-font-family-mono);
}

.metric-card--emerald {
  background: linear-gradient(rgba(34, 197, 94, 0.14), rgba(34, 197, 94, 0.14)), var(--n-color);
}

.metric-card--emerald .metric-card__icon {
  background: #22c55e;
}

.metric-card--violet {
  background: linear-gradient(rgba(14, 165, 233, 0.14), rgba(14, 165, 233, 0.14)), var(--n-color);
}

.metric-card--violet .metric-card__icon {
  background: #0ea5e9;
}

.metric-card--amber {
  background: linear-gradient(rgba(245, 158, 11, 0.16), rgba(245, 158, 11, 0.16)), var(--n-color);
}

.metric-card--amber .metric-card__icon {
  background: #f59e0b;
}

.metric-card--rose {
  background: linear-gradient(rgba(239, 68, 68, 0.13), rgba(239, 68, 68, 0.13)), var(--n-color);
}

.metric-card--rose .metric-card__icon {
  background: #ef4444;
}

.dashboard-panels {
  display: grid;
  grid-template-columns: minmax(0, 0.95fr) minmax(0, 1.65fr);
  gap: 14px;
}

.panel-card {
  border: var(--comic-border) solid var(--comic-ink);
  box-shadow: var(--comic-shadow);
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 18px;
  margin-bottom: 20px;
}

.panel-title {
  font-size: 18px;
  font-weight: 800;
  letter-spacing: -0.03em;
  color: var(--n-text-color);
}

.panel-subtitle {
  margin-top: 6px;
  font-size: 13px;
  color: var(--n-text-color-3);
}

.usage-badge {
  flex-shrink: 0;
  padding: 8px 12px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  border: 1px solid transparent;
}

.usage-badge--healthy {
  color: #059669;
  background: rgba(16, 185, 129, 0.12);
  border-color: rgba(16, 185, 129, 0.18);
}

.usage-badge--warm {
  color: #d97706;
  background: rgba(245, 158, 11, 0.14);
  border-color: rgba(245, 158, 11, 0.22);
}

.usage-badge--critical {
  color: #dc2626;
  background: rgba(239, 68, 68, 0.12);
  border-color: rgba(239, 68, 68, 0.2);
}

.usage-total {
  display: flex;
  align-items: flex-end;
  gap: 8px;
  margin-bottom: 18px;
  font-size: 40px;
  line-height: 1;
  font-weight: 800;
  letter-spacing: -0.05em;
  color: var(--n-text-color);
  font-family: var(--comic-font-display);
}

.usage-total span {
  margin-bottom: 4px;
  font-size: 15px;
  color: var(--n-text-color-3);
  font-family: var(--n-font-family-mono);
}

.usage-progress {
  height: 18px;
  padding: 2px;
  margin-bottom: 18px;
  border-radius: 8px;
  background: rgba(14, 165, 233, 0.1);
  border: 2px solid var(--comic-ink);
}

.usage-progress__fill {
  height: 100%;
  border-radius: 4px;
  transition: width 0.35s ease;
}

.usage-progress__fill.usage-badge--healthy {
  background: #22c55e;
}

.usage-progress__fill.usage-badge--warm {
  background: #f59e0b;
}

.usage-progress__fill.usage-badge--critical {
  background: #ef4444;
}

.usage-split {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.usage-split__item {
  padding: 14px;
  border-radius: 10px;
  background: rgba(14, 165, 233, 0.06);
  border: 2px solid var(--comic-ink);
}

.usage-split__label {
  display: block;
  margin-bottom: 8px;
  font-size: 12px;
  font-weight: 700;
  color: var(--n-text-color-3);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.usage-split__value {
  font-size: 22px;
  line-height: 1;
  letter-spacing: -0.03em;
}

.chart-card__header {
  margin-bottom: 16px;
}

.chart-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}

.chart-summary__item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 999px;
  font-size: 12px;
  background: rgba(14, 165, 233, 0.08);
  border: 1.5px solid var(--comic-ink);
  color: var(--n-text-color-2);
}

.chart-summary__item--violet {
  background: rgba(14, 165, 233, 0.1);
}

.chart-summary__item--emerald {
  background: rgba(29, 78, 216, 0.12);
}

.chart-summary__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

.chart-summary__item--violet .chart-summary__dot {
  color: #0ea5e9;
}

.chart-summary__item--emerald .chart-summary__dot {
  color: #1d4ed8;
}

.chart-summary__label {
  color: var(--n-text-color-3);
}

.chart-summary__value {
  color: var(--n-text-color);
  font-family: var(--n-font-family-mono);
}

.chart-controls {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  flex-shrink: 0;
}

.chart-type-toggle {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 700;
  color: var(--n-text-color-2);
  background: rgba(14, 165, 233, 0.08);
  border: 2px solid var(--comic-ink);
  border-radius: 999px;
  cursor: pointer;
  transition: background 0.18s ease, transform 0.1s ease;
}

.chart-type-toggle:hover {
  background: rgba(14, 165, 233, 0.16);
}

.chart-type-toggle:active {
  transform: translateY(1px);
}

.chart-type-toggle:focus-visible {
  outline: 2px solid #0ea5e9;
  outline-offset: 2px;
}

.chart-tabs {
  width: 220px;
  flex-shrink: 0;
}

@media (max-width: 1100px) {
  .dashboard-panels {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 768px) {
  .page-title {
    font-size: 22px;
  }

  .metric-card__value {
    font-size: 28px;
  }

  .panel-header {
    flex-direction: column;
    align-items: stretch;
  }

  .chart-tabs {
    width: 100%;
  }

  .usage-total {
    font-size: 32px;
  }

  .chart-canvas {
    height: 320px;
  }
}

@media (max-width: 640px) {
  .page-caption {
    font-size: 13px;
  }

  /* 手机端指标卡两列并排：收紧间距，数值与补充文字过长时允许换行 */
  .metric-card__header {
    margin-bottom: 12px;
    gap: 8px;
  }

  .metric-card__label {
    letter-spacing: 0.02em;
  }

  .metric-card__value-row {
    flex-wrap: wrap;
    gap: 2px 8px;
  }

  .metric-card__icon {
    width: 30px;
    height: 30px;
    flex-shrink: 0;
    border-radius: 8px;
    font-size: 15px;
  }

  .metric-card__value {
    font-size: 26px;
  }

  .metric-card__supporting {
    font-size: 12px;
  }

  .usage-split__item {
    padding: 10px;
  }

  .usage-total {
    flex-direction: column;
    align-items: flex-start;
    gap: 6px;
  }

  .usage-total span {
    margin-bottom: 0;
  }

  .chart-summary {
    gap: 6px;
  }

  .chart-summary__item {
    width: 100%;
    justify-content: space-between;
    border-radius: 14px;
  }

  .chart-canvas {
    height: 280px;
  }
}
</style>
