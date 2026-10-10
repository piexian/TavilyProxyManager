<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NCard, NGi, NGrid, NIcon, NTabPane, NTabs } from "naive-ui";
import {
  BarChartOutline,
  BatteryFullOutline,
  CloudUploadOutline,
  KeyOutline,
  PulseOutline,
  TimeOutline,
  TrendingUpOutline,
} from "@vicons/ionicons5";
import RequestChart from "../RequestChart.vue";
import { locale, t } from "../../i18n";
import type { PortalOverview, TimeSeries } from "../../types";

const props = defineProps<{
  overview: PortalOverview | null;
  timeseries: TimeSeries | null;
}>();

const emit = defineEmits<{
  (e: "change-granularity", value: "hour" | "day" | "month"): void;
}>();

const granularity = ref<"hour" | "day" | "month">("hour");
const chartType = ref<"bar" | "line">("bar");

function toggleChartType() {
  chartType.value = chartType.value === "bar" ? "line" : "bar";
}

watch(granularity, (g) => emit("change-granularity", g));

const numberFormatter = computed(
  () => new Intl.NumberFormat(locale.value === "zh-CN" ? "zh-CN" : "en-US"),
);
function formatNumber(value: number): string {
  return numberFormatter.value.format(value);
}

const usagePercent = computed(() => {
  const o = props.overview;
  if (!o || o.total_quota <= 0) return 0;
  return Math.max(0, Math.min(100, Math.round((o.used_quota / o.total_quota) * 100)));
});

const metricCards = computed(() => {
  const o = props.overview;
  return [
    {
      key: "keys",
      label: t("portal.overview.myKeys"),
      value: formatNumber(o?.key_count ?? 0),
      supporting: "",
      icon: KeyOutline,
      tone: "violet",
    },
    {
      key: "remaining",
      label: t("portal.overview.remaining"),
      value: formatNumber(o?.remaining ?? 0),
      supporting: `/ ${formatNumber(o?.total_quota ?? 0)}`,
      icon: BatteryFullOutline,
      tone: "emerald",
    },
    {
      key: "used",
      label: t("portal.overview.used"),
      value: formatNumber(o?.used_quota ?? 0),
      supporting: `${usagePercent.value}%`,
      icon: CloudUploadOutline,
      tone: "amber",
    },
    {
      key: "today",
      label: t("portal.overview.todayRequests"),
      value: formatNumber(o?.today_requests ?? 0),
      supporting: "",
      icon: PulseOutline,
      tone: "rose",
    },
    {
      key: "total",
      label: t("portal.overview.totalRequests"),
      value: formatNumber(o?.total_requests ?? 0),
      supporting: "",
      icon: TimeOutline,
      tone: "violet",
    },
  ];
});

function localizeSeriesName(name: string): string {
  if (name === "Served By My Keys") return t("portal.timeseries.servedByMyKeys");
  if (name === "My Requests") return t("portal.timeseries.myRequests");
  return name;
}

const chartSummaries = computed(() => {
  if (!props.timeseries) return [];
  return props.timeseries.series.map((series, idx) => ({
    name: localizeSeriesName(series.name),
    value: formatNumber(series.data.reduce((a, b) => a + b, 0)),
    tone: idx === 0 ? "violet" : "amber",
  }));
});
</script>

<template>
  <div class="portal-overview">
    <n-grid cols="1 s:2 m:5" responsive="screen" :x-gap="12" :y-gap="12">
      <n-gi v-for="card in metricCards" :key="card.key">
        <n-card size="small" class="metric-card" :class="`metric-card--${card.tone}`">
          <div class="metric-card__header">
            <span class="metric-card__label">{{ card.label }}</span>
            <div class="metric-card__icon">
              <n-icon :component="card.icon" />
            </div>
          </div>
          <div class="metric-card__value-row">
            <span class="metric-card__value">{{ card.value }}</span>
            <span v-if="card.supporting" class="metric-card__supporting">{{ card.supporting }}</span>
          </div>
        </n-card>
      </n-gi>
    </n-grid>

    <n-card size="small" class="panel-card chart-card">
      <div class="panel-header chart-card__header">
        <div>
          <div class="panel-title">{{ t("portal.overview.chartTitle") }}</div>
          <div class="panel-subtitle">{{ t("portal.overview.chartSubtitle") }}</div>
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
            <n-icon :component="chartType === 'bar' ? BarChartOutline : TrendingUpOutline" />
            <span>{{ chartType === "bar" ? t("portal.chartType.bar") : t("portal.chartType.line") }}</span>
          </button>
          <n-tabs v-model:value="granularity" type="segment" size="small" class="chart-tabs">
            <n-tab-pane name="hour" :tab="t('dashboard.requestAnalytics.hour')" />
            <n-tab-pane name="day" :tab="t('dashboard.requestAnalytics.day')" />
            <n-tab-pane name="month" :tab="t('dashboard.requestAnalytics.month')" />
          </n-tabs>
        </div>
      </div>

      <RequestChart :timeseries="timeseries" :chart-type="chartType" :granularity="granularity" />
    </n-card>
  </div>
</template>

<style scoped>
.portal-overview {
  display: flex;
  flex-direction: column;
  gap: 16px;
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
  font-size: 34px;
  line-height: 1;
  font-weight: 800;
  letter-spacing: -0.04em;
  font-family: var(--comic-font-display);
}

.metric-card__supporting {
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

.panel-card {
  border: var(--comic-border) solid var(--comic-ink);
  box-shadow: var(--comic-shadow);
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 18px;
  margin-bottom: 16px;
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
.chart-summary__item--amber {
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
.chart-summary__item--amber .chart-summary__dot {
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

@media (max-width: 768px) {
  .panel-header {
    flex-direction: column;
    align-items: stretch;
  }
  .chart-tabs {
    width: 100%;
  }
  .metric-card__value {
    font-size: 28px;
  }
}
</style>
