<script setup lang="ts">
import {
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import * as echarts from "echarts";
import { locale, t } from "../i18n";
import type { TimeSeries } from "../types";

const props = withDefaults(
  defineProps<{
    timeseries: TimeSeries | null;
    chartType: "bar" | "line";
    granularity: "hour" | "day" | "month";
    height?: string;
  }>(),
  { height: "360px" },
);

const chartEl = ref<HTMLDivElement | null>(null);
let chart: echarts.ECharts | null = null;

const numberFormatter = () =>
  new Intl.NumberFormat(locale.value === "zh-CN" ? "zh-CN" : "en-US");

function formatNumber(value: number): string {
  return numberFormatter().format(value);
}

// 兼容管理员与门户两套后端序列名。
function localizeSeriesName(name: string): string {
  switch (name) {
    case "All Requests":
      return t("dashboard.timeseries.allRequests");
    case "Search":
      return t("dashboard.timeseries.search");
    case "Served By My Keys":
      return t("portal.timeseries.servedByMyKeys");
    case "My Requests":
      return t("portal.timeseries.myRequests");
    default:
      return name;
  }
}

function isDarkMode(): boolean {
  return (
    document.documentElement.getAttribute("data-theme") === "dark" ||
    localStorage.getItem("theme") === "dark"
  );
}

function ensureChart() {
  if (!chartEl.value || chart) return;
  chart = echarts.init(chartEl.value);
  window.addEventListener("resize", onResize);
}

function onResize() {
  chart?.resize();
}

function renderChart() {
  ensureChart();
  if (!chart || !props.timeseries) return;

  const ts = props.timeseries;
  const isDark = isDarkMode();
  const inkColor = isDark ? "#04111f" : "#12263a";
  const seriesColors = ["#0ea5e9", "#1d4ed8"]; // 天蓝 / 深蓝
  const textColor = isDark ? "#e2e8f0" : "#12263a";
  const mutedTextColor = isDark ? "#94a3b8" : "#64748b";
  const splitLineColor = isDark ? "rgba(255,255,255,0.10)" : "rgba(18,38,58,0.12)";
  const shadowColor = isDark ? "rgba(56,189,248,0.18)" : "rgba(14,165,233,0.14)";
  const tooltipBg = isDark ? "#1e293b" : "#ffffff";
  const comicFont =
    '"Baloo 2", "Comic Neue", "PingFang SC", "Microsoft YaHei", system-ui, sans-serif';

  const buildSeries = (series: { name: string; data: number[] }, idx: number) => {
    const localizedName = localizeSeriesName(series.name);
    const fill = seriesColors[idx % seriesColors.length];
    if (props.chartType === "line") {
      return {
        name: localizedName,
        type: "line" as const,
        smooth: true,
        symbol: "circle",
        symbolSize: 7,
        z: 2,
        lineStyle: { color: fill, width: 3 },
        itemStyle: { color: fill, borderColor: inkColor, borderWidth: 2 },
        areaStyle: { color: fill, opacity: 0.12 },
        emphasis: { focus: "series" as const },
        data: series.data,
      };
    }
    return {
      name: localizedName,
      type: "bar" as const,
      barGap: "20%",
      barCategoryGap: props.granularity === "month" ? "40%" : "34%",
      barMaxWidth: props.granularity === "month" ? 30 : 20,
      z: 2,
      itemStyle: {
        color: fill,
        borderColor: inkColor,
        borderWidth: 2,
        borderRadius: [8, 8, 0, 0] as [number, number, number, number],
      },
      emphasis: {
        focus: "series" as const,
        itemStyle: {
          color: fill,
          borderColor: inkColor,
          borderWidth: 2,
          shadowBlur: 0,
          shadowOffsetX: 3,
          shadowOffsetY: 3,
          shadowColor: inkColor,
        },
      },
      data: series.data,
    };
  };

  chart.setOption(
    {
      animationDuration: 450,
      backgroundColor: "transparent",
      tooltip: {
        trigger: "axis",
        axisPointer: {
          type: props.chartType === "line" ? "line" : "shadow",
          shadowStyle: { color: shadowColor },
          lineStyle: { color: shadowColor, width: 2 },
        },
        backgroundColor: tooltipBg,
        borderColor: inkColor,
        borderWidth: 2,
        textStyle: { color: textColor, fontFamily: comicFont, fontWeight: 600 },
        extraCssText: `box-shadow: 4px 4px 0 ${inkColor}; border-radius: 12px; padding: 10px 12px;`,
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
        boundaryGap: props.chartType !== "line",
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
        axisLabel: { color: mutedTextColor, margin: 12, fontSize: 12 },
        splitLine: { lineStyle: { color: splitLineColor, type: "dashed" } },
      },
      series: ts.series.map(buildSeries),
      textStyle: { color: textColor, fontFamily: comicFont },
    },
    { notMerge: true },
  );
}

watch(
  () => [props.timeseries, props.chartType, props.granularity, locale.value],
  async () => {
    renderChart();
  },
  { deep: true },
);

onMounted(() => {
  ensureChart();
  renderChart();
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", onResize);
  chart?.dispose();
  chart = null;
});
</script>

<template>
  <div class="chart-shell">
    <div ref="chartEl" class="chart-canvas" :style="{ height }" />
  </div>
</template>

<style scoped>
.chart-shell {
  border-radius: 12px;
  padding: 10px;
  background: rgba(14, 165, 233, 0.04);
  border: 2px solid var(--comic-ink);
}

.chart-canvas {
  width: 100%;
}
</style>
