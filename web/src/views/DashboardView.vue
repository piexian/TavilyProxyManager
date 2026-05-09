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

    <n-grid cols="1 s:2 m:5" responsive="screen" :x-gap="12" :y-gap="12">
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

        <div class="chart-shell">
          <div ref="chartEl" class="chart-canvas" />
        </div>
      </n-card>
    </div>
  </n-space>
</template>

<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import * as echarts from "echarts";
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
  BatteryFullOutline,
  CloudUploadOutline,
  KeyOutline,
  PulseOutline,
  RefreshOutline,
  ServerOutline,
} from "@vicons/ionicons5";
import { api } from "../api/client";
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

const chartEl = ref<HTMLDivElement | null>(null);
let chart: echarts.ECharts | null = null;

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
    renderChart();
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("dashboard.errors.loadChart"));
  } finally {
    loadingChart.value = false;
  }
}

function ensureChart() {
  if (!chartEl.value) return;
  if (chart) return;
  chart = echarts.init(chartEl.value);
  window.addEventListener("resize", onResize);
}

function onResize() {
  chart?.resize();
}

function localizeSeriesName(name: string): string {
  if (name === "All Requests") return t("dashboard.timeseries.allRequests");
  if (name === "Search") return t("dashboard.timeseries.search");
  return name;
}

function isDarkMode(): boolean {
  return (
    document.documentElement.classList.contains("dark") ||
    localStorage.getItem("theme") === "dark"
  );
}

function renderChart() {
  ensureChart();
  if (!chart || !timeseries.value) return;

  const ts = timeseries.value;
  const isDark = isDarkMode();
  const primaryBarTop = "#818cf8";
  const primaryBarBottom = "#6366f1";
  const secondaryBarTop = "#34d399";
  const secondaryBarBottom = "#10b981";
  const textColor = isDark ? "#c7cad1" : "#697386";
  const mutedTextColor = isDark ? "#8f96a3" : "#98a2b3";
  const splitLineColor = isDark ? "rgba(255,255,255,0.08)" : "rgba(99,102,241,0.08)";
  const shadowColor = isDark ? "rgba(255,255,255,0.04)" : "rgba(99,102,241,0.06)";

  chart.setOption(
    {
      animationDuration: 450,
      backgroundColor: "transparent",
      tooltip: {
        trigger: "axis",
        axisPointer: {
          type: "shadow",
          shadowStyle: { color: shadowColor },
        },
        backgroundColor: isDark ? "#161b26" : "#ffffff",
        borderWidth: 0,
        textStyle: { color: isDark ? "#f5f7fb" : "#101828" },
        extraCssText:
          "box-shadow: 0 16px 40px rgba(15, 23, 42, 0.16); border-radius: 14px; padding: 10px 12px;",
        formatter: (params: any) => {
          const rows = Array.isArray(params) ? params : [params];
          const title = rows[0]?.axisValueLabel ?? "";
          return [
            `<div style="margin-bottom:8px;font-weight:700;">${title}</div>`,
            ...rows.map(
              (row: any) =>
                `${row.marker}<span style="display:inline-block;min-width:112px;">${row.seriesName}</span><strong>${formatNumber(Number(row.value ?? 0))}</strong>`,
            ),
          ].join("<br/>");
        },
      },
      grid: { left: 12, right: 12, top: 18, bottom: 8, containLabel: true },
      xAxis: {
        type: "category",
        data: ts.labels,
        boundaryGap: true,
        axisTick: { show: false },
        axisLine: { show: false },
        axisLabel: {
          color: mutedTextColor,
          margin: 12,
          fontSize: 12,
          hideOverlap: true,
        },
      },
      yAxis: {
        type: "value",
        minInterval: 1,
        splitNumber: 4,
        axisLine: { show: false },
        axisTick: { show: false },
        axisLabel: {
          color: mutedTextColor,
          margin: 12,
          fontSize: 12,
        },
        splitLine: {
          lineStyle: {
            color: splitLineColor,
            type: "dashed",
          },
        },
      },
      series: ts.series.map((series, idx) => {
        const localizedName = localizeSeriesName(series.name);
        const gradient =
          idx === 0
            ? [primaryBarTop, primaryBarBottom]
            : [secondaryBarTop, secondaryBarBottom];
        const seriesShadow =
          idx === 0
            ? "rgba(99, 102, 241, 0.24)"
            : "rgba(16, 185, 129, 0.24)";
        return {
          name: localizedName,
          type: "bar",
          barGap: "20%",
          barCategoryGap: granularity.value === "month" ? "40%" : "34%",
          barMaxWidth: granularity.value === "month" ? 26 : 16,
          z: 2,
          itemStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: gradient[0] },
              { offset: 1, color: gradient[1] },
            ]),
            borderRadius: [10, 10, 4, 4],
          },
          emphasis: {
            focus: "series",
            itemStyle: {
              shadowBlur: 18,
              shadowColor: seriesShadow,
            },
          },
          data: series.data,
        };
      }),
      textStyle: {
        color: textColor,
      },
    },
    { notMerge: true },
  );
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
  await nextTick();
  await refreshTimeSeries();
});

watch(locale, async () => {
  await nextTick();
  renderChart();
});

onMounted(async () => {
  await refreshStats();
  await nextTick();
  ensureChart();
  await refreshTimeSeries();
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", onResize);
  chart?.dispose();
  chart = null;
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
  border: 1px solid rgba(128, 128, 128, 0.16);
  transition: transform 0.22s ease, box-shadow 0.22s ease, border-color 0.22s ease;
}

.metric-card:hover {
  transform: translateY(-3px);
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.08);
}

.metric-card__shine {
  position: absolute;
  inset: -30% auto auto -8%;
  width: 160px;
  height: 160px;
  border-radius: 50%;
  opacity: 0.8;
  filter: blur(12px);
  pointer-events: none;
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
  border-radius: 12px;
  font-size: 18px;
  color: #fff;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.18);
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
  background: linear-gradient(180deg, rgba(16, 185, 129, 0.1), rgba(16, 185, 129, 0.02)), var(--n-color);
}

.metric-card--emerald .metric-card__shine {
  background: rgba(16, 185, 129, 0.2);
}

.metric-card--emerald .metric-card__icon {
  background: linear-gradient(135deg, #10b981, #34d399);
}

.metric-card--violet {
  background: linear-gradient(180deg, rgba(99, 102, 241, 0.1), rgba(99, 102, 241, 0.02)), var(--n-color);
}

.metric-card--violet .metric-card__shine {
  background: rgba(129, 140, 248, 0.22);
}

.metric-card--violet .metric-card__icon {
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
}

.metric-card--amber {
  background: linear-gradient(180deg, rgba(245, 158, 11, 0.12), rgba(245, 158, 11, 0.03)), var(--n-color);
}

.metric-card--amber .metric-card__shine {
  background: rgba(251, 191, 36, 0.22);
}

.metric-card--amber .metric-card__icon {
  background: linear-gradient(135deg, #f59e0b, #fbbf24);
}

.metric-card--rose {
  background: linear-gradient(180deg, rgba(239, 68, 68, 0.11), rgba(239, 68, 68, 0.02)), var(--n-color);
}

.metric-card--rose .metric-card__shine {
  background: rgba(248, 113, 113, 0.2);
}

.metric-card--rose .metric-card__icon {
  background: linear-gradient(135deg, #ef4444, #fb7185);
}

.dashboard-panels {
  display: grid;
  grid-template-columns: minmax(0, 0.95fr) minmax(0, 1.65fr);
  gap: 14px;
}

.panel-card {
  border: 1px solid rgba(128, 128, 128, 0.16);
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
}

.usage-total span {
  margin-bottom: 4px;
  font-size: 15px;
  color: var(--n-text-color-3);
  font-family: var(--n-font-family-mono);
}

.usage-progress {
  height: 16px;
  padding: 2px;
  margin-bottom: 18px;
  border-radius: 999px;
  background: rgba(99, 102, 241, 0.08);
  box-shadow: inset 0 1px 2px rgba(15, 23, 42, 0.08);
}

.usage-progress__fill {
  height: 100%;
  border-radius: inherit;
  transition: width 0.35s ease;
}

.usage-progress__fill.usage-badge--healthy {
  background: linear-gradient(90deg, #10b981, #34d399);
}

.usage-progress__fill.usage-badge--warm {
  background: linear-gradient(90deg, #f59e0b, #fbbf24);
}

.usage-progress__fill.usage-badge--critical {
  background: linear-gradient(90deg, #ef4444, #fb7185);
}

.usage-split {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.usage-split__item {
  padding: 14px;
  border-radius: 14px;
  background: rgba(99, 102, 241, 0.04);
  border: 1px solid rgba(99, 102, 241, 0.08);
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
  background: rgba(99, 102, 241, 0.06);
  color: var(--n-text-color-2);
}

.chart-summary__item--violet {
  background: rgba(99, 102, 241, 0.08);
}

.chart-summary__item--emerald {
  background: rgba(16, 185, 129, 0.1);
}

.chart-summary__dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

.chart-summary__item--violet .chart-summary__dot {
  color: #6366f1;
}

.chart-summary__item--emerald .chart-summary__dot {
  color: #10b981;
}

.chart-summary__label {
  color: var(--n-text-color-3);
}

.chart-summary__value {
  color: var(--n-text-color);
  font-family: var(--n-font-family-mono);
}

.chart-tabs {
  width: 220px;
  flex-shrink: 0;
}

.chart-shell {
  border-radius: 20px;
  padding: 10px;
  background:
    linear-gradient(180deg, rgba(99, 102, 241, 0.04), rgba(99, 102, 241, 0)),
    rgba(99, 102, 241, 0.015);
  border: 1px solid rgba(99, 102, 241, 0.08);
}

.chart-canvas {
  width: 100%;
  height: 360px;
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

  .metric-card__header {
    margin-bottom: 18px;
  }

  .metric-card__icon {
    width: 34px;
    height: 34px;
    border-radius: 10px;
    font-size: 16px;
  }

  .metric-card__value {
    font-size: 26px;
  }

  .metric-card__supporting {
    font-size: 12px;
  }

  .usage-split {
    grid-template-columns: 1fr;
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
