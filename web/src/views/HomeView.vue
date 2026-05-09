<template>
  <div class="home-view">
    <div class="home-header">
      <h1 class="home-title">{{ t("home.title") }}</h1>
      <p class="home-desc">{{ t("home.description") }}</p>
      <div class="endpoint-container">
        <div class="endpoint-badge">
          <span class="label">Endpoint:</span>
          <code class="url">{{ baseUrl }}</code>
          <n-button quaternary circle size="tiny" @click="copy(baseUrl)">
            <template #icon><n-icon :component="CopyOutline" /></template>
          </n-button>
        </div>
      </div>
    </div>

    <div class="section-card guide-card">
      <n-tabs type="segment" animated class="guide-tabs">
        <n-tab-pane
          v-for="item in templates"
          :key="item.key"
          :name="item.key"
          :tab="item.tab"
        >
          <div class="pane-content">
            <div class="pane-header">
              <h3 class="pane-title">{{ item.title }}</h3>
              <p class="pane-desc" v-if="item.desc">{{ item.desc }}</p>
            </div>

            <div class="code-wrapper">
              <pre class="code-block"><code>{{ item.code }}</code></pre>
              <div class="copy-floating">
                <n-button quaternary size="small" @click="copy(item.code)">
                  <template #icon><n-icon :component="CopyOutline" /></template>
                  <span v-if="item.code.length < 50">{{ t("home.copy") }}</span>
                </n-button>
              </div>
            </div>

            <div class="pane-hint" v-if="item.hint">
              <div class="hint-content">{{ item.hint }}</div>
            </div>
          </div>
        </n-tab-pane>
      </n-tabs>
    </div>

    <div class="section-card quota-card">
      <div class="quota-card-top">
        <div>
          <h2 class="quota-title">{{ t("home.quotaTitle") }}</h2>
          <p class="quota-desc">
            {{ t("home.quotaDesc", { count: formatMetric(activeKeyCount) }) }}
          </p>
        </div>
        <div class="quota-pill" :class="{ 'quota-pill-muted': statsUnavailable }">
          {{ statsUnavailable ? t("home.poolUnavailable") : t("home.poolLive") }}
        </div>
      </div>

      <div class="quota-progress-panel">
        <div class="quota-progress-header">
          <div>
            <div class="quota-progress-label">{{ t("home.availableNow") }}</div>
            <strong class="quota-progress-value">{{ formatMetric(totalRemaining) }}</strong>
          </div>
          <div class="quota-progress-side">
            <div class="quota-mini">
              <span class="quota-mini__label">{{ t("home.totalQuota") }}</span>
              <strong class="quota-mini__value">{{ formatMetric(totalQuota) }}</strong>
            </div>
          </div>
        </div>
        <div class="quota-progress-meta">
          <span>{{ t("home.availableNow") }}</span>
          <span>{{ remainingPercent }}%</span>
        </div>
        <div class="quota-progress-track" aria-hidden="true">
          <div class="quota-progress-fill" :style="{ width: `${remainingPercent}%` }"></div>
          <div class="quota-progress-glow"></div>
        </div>
        <div class="quota-progress-foot">
          <span>{{ t("home.usedQuota", { count: formatMetric(totalUsed) }) }}</span>
          <span>{{ t("home.activeKeys", { count: formatMetric(activeKeyCount) }) }}</span>
        </div>
      </div>
    </div>

    <KeyDonation @quota-updated="loadPoolStats" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { NButton, NIcon, NTabPane, NTabs, useMessage } from "naive-ui";
import { CopyOutline } from "@vicons/ionicons5";
import { api } from "../api/client";
import { writeClipboardText } from "../utils/clipboard";
import { locale, t } from "../i18n";
import type { PoolStats } from "../types";
import KeyDonation from "../components/KeyDonation.vue";

const message = useMessage();
const baseUrl = computed(() => window.location.origin);
const poolStats = ref<PoolStats | null>(null);
const statsLoaded = ref(false);
const statsUnavailable = ref(false);

const numberFormatter = computed(
  () => new Intl.NumberFormat(locale.value === "zh-CN" ? "zh-CN" : "en-US"),
);

const totalQuota = computed(() => poolStats.value?.total_quota ?? 0);
const totalRemaining = computed(() => poolStats.value?.total_remaining ?? 0);
const totalUsed = computed(() => poolStats.value?.total_used ?? 0);
const activeKeyCount = computed(() => poolStats.value?.active_key_count ?? 0);

const remainingPercent = computed(() => {
  const total = totalQuota.value;
  if (total <= 0) return 0;
  return Math.max(0, Math.min(100, Math.round((totalRemaining.value / total) * 100)));
});

async function copy(text: string) {
  try {
    await writeClipboardText(text);
    message.success(t("common.copiedToClipboard"));
  } catch {
    message.error(t("common.copyFailed"));
  }
}

function formatValue(value: number): string {
  return numberFormatter.value.format(value);
}

function formatMetric(value: number): string {
  if (poolStats.value === null && (!statsLoaded.value || statsUnavailable.value)) {
    return "--";
  }
  return formatValue(value);
}

async function loadPoolStats() {
  try {
    const { data } = await api.get<PoolStats>("/api/public/stats");
    poolStats.value = data;
    statsUnavailable.value = false;
  } catch {
    statsUnavailable.value = true;
  } finally {
    statsLoaded.value = true;
  }
}

const templates = computed(() => {
  const url = baseUrl.value;
  return [
    {
      key: "mcp",
      tab: "MCP",
      title: t("home.mcp.title"),
      desc: t("home.mcp.desc"),
      hint: t("home.mcp.hint"),
      code: JSON.stringify(
        {
          mcpServers: {
            "tavily-proxy": {
              url: `${url}/mcp`,
              headers: { Authorization: "Bearer YOUR_API_KEY" },
            },
          },
        },
        null,
        2,
      ),
    },
    {
      key: "claude-code",
      tab: "Claude Code",
      title: t("home.claudeCode.title"),
      desc: t("home.claudeCode.desc"),
      hint: "",
      code: `claude mcp add tavily-proxy \\
  --transport http \\
  ${url}/mcp \\
  --header "Authorization: Bearer YOUR_API_KEY"`,
    },
    {
      key: "sse",
      tab: "SSE",
      title: t("home.sse.title"),
      desc: t("home.sse.desc"),
      hint: t("home.sse.hint"),
      code: JSON.stringify(
        {
          mcpServers: {
            "tavily-proxy": {
              url: `${url}/sse/`,
              transport: "sse",
              headers: { Authorization: "Bearer YOUR_API_KEY" },
            },
          },
        },
        null,
        2,
      ),
    },
    {
      key: "api",
      tab: "REST API",
      title: t("home.api.title"),
      desc: t("home.api.desc"),
      hint: "",
      code: `curl -X POST ${url}/search \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"query": "latest AI news"}'`,
    },
    {
      key: "astrbot",
      tab: "AstrBot",
      title: t("home.astrbot.title"),
      desc: t("home.astrbot.desc"),
      hint: t("home.astrbot.hint"),
      code: JSON.stringify(
        {
          transport: "streamable_http",
          url: `${url}/mcp`,
          headers: { Authorization: "Bearer YOUR_API_KEY" },
          timeout: 5,
          sse_read_timeout: 300,
        },
        null,
        2,
      ),
    },
  ];
});

onMounted(() => {
  void loadPoolStats();
});
</script>

<style scoped>
.home-view {
  max-width: 720px;
  margin: 0 auto;
  padding: 40px 20px;
}

.home-header {
  text-align: center;
  margin-bottom: 48px;
}

.home-title {
  font-size: 32px;
  font-weight: 800;
  margin: 0 0 12px;
  color: var(--n-text-color);
  letter-spacing: -0.5px;
}

.home-desc {
  font-size: 16px;
  color: var(--n-text-color-3);
  margin: 0 0 24px;
  line-height: 1.6;
}

.endpoint-container {
  display: flex;
  justify-content: center;
}

.endpoint-badge {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  background-color: var(--n-code-color);
  border-radius: 20px;
  font-size: 13px;
  border: 1px solid var(--n-border-color);
}

.endpoint-badge .label {
  color: var(--n-text-color-3);
  font-weight: 500;
}

.endpoint-badge .url {
  color: var(--n-primary-color);
  font-weight: 600;
  font-family: var(--n-font-family-mono);
}

.guide-tabs :deep(.n-tabs-tabpane) {
  padding-top: 24px;
}

.section-card {
  padding: 28px;
  border: 1px solid rgba(128, 128, 128, 0.2);
  border-radius: 16px;
  margin-bottom: 32px;
}

.quota-card {
  position: relative;
  overflow: hidden;
  background:
    radial-gradient(circle at top right, rgba(99, 102, 241, 0.16), transparent 32%),
    radial-gradient(circle at bottom left, rgba(14, 165, 233, 0.12), transparent 30%),
    linear-gradient(180deg, rgba(99, 102, 241, 0.05), rgba(99, 102, 241, 0.015));
}

.quota-card::after {
  content: "";
  position: absolute;
  inset: auto -10% -55% auto;
  width: 220px;
  height: 220px;
  background: radial-gradient(circle, rgba(129, 140, 248, 0.18), transparent 68%);
  pointer-events: none;
}

.quota-card-top {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 22px;
}

.quota-title {
  margin: 0 0 8px;
  font-size: 24px;
  font-weight: 800;
  letter-spacing: -0.03em;
  color: var(--n-text-color);
}

.quota-desc {
  margin: 0;
  max-width: 480px;
  font-size: 14px;
  line-height: 1.6;
  color: var(--n-text-color-3);
}

.quota-pill {
  flex-shrink: 0;
  padding: 8px 12px;
  border-radius: 999px;
  border: 1px solid rgba(99, 102, 241, 0.16);
  background: rgba(99, 102, 241, 0.1);
  color: var(--n-primary-color);
  font-size: 12px;
  font-weight: 700;
}

.quota-pill-muted {
  color: var(--n-text-color-3);
  background: rgba(128, 128, 128, 0.08);
  border-color: rgba(128, 128, 128, 0.18);
}

.quota-progress-panel {
  position: relative;
  z-index: 1;
  padding: 18px;
  border-radius: 14px;
  border: 1px solid rgba(128, 128, 128, 0.16);
  background:
    linear-gradient(180deg, rgba(99, 102, 241, 0.04), rgba(99, 102, 241, 0)),
    var(--n-color);
  backdrop-filter: blur(14px);
}

.quota-progress-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 14px;
}

.quota-progress-label {
  margin-bottom: 8px;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--n-text-color-3);
}

.quota-progress-value {
  display: block;
  font-size: 34px;
  line-height: 1;
  letter-spacing: -0.05em;
  color: var(--n-text-color);
}

.quota-progress-side {
  width: min(100%, 180px);
}

.quota-mini {
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid rgba(128, 128, 128, 0.16);
  background: rgba(99, 102, 241, 0.05);
}

.quota-mini--accent {
  border-color: rgba(16, 185, 129, 0.18);
  background: rgba(16, 185, 129, 0.08);
}

.quota-mini__label {
  display: block;
  margin-bottom: 6px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--n-text-color-3);
}

.quota-mini__value {
  display: block;
  font-size: 20px;
  line-height: 1;
  color: var(--n-text-color);
}

.quota-progress-meta,
.quota-progress-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
}

.quota-progress-meta {
  margin-bottom: 12px;
  font-weight: 600;
  color: var(--n-text-color-2);
}

.quota-progress-track {
  position: relative;
  height: 16px;
  border-radius: 999px;
  background: rgba(99, 102, 241, 0.08);
  overflow: hidden;
  box-shadow: inset 0 1px 2px rgba(15, 23, 42, 0.06);
}

.quota-progress-fill {
  position: relative;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #10b981 0%, #34d399 48%, #60a5fa 100%);
  transition: width 0.35s ease;
}

.quota-progress-glow {
  position: absolute;
  inset: 2px auto 2px 12px;
  width: 36%;
  border-radius: 999px;
  background: linear-gradient(90deg, rgba(255, 255, 255, 0.45), rgba(255, 255, 255, 0));
  mix-blend-mode: screen;
  pointer-events: none;
}

.quota-progress-foot {
  margin-top: 12px;
  color: var(--n-text-color-3);
}

.pane-content {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.pane-header {
  margin-bottom: 4px;
}

.pane-title {
  font-size: 20px;
  font-weight: 700;
  margin: 0 0 8px;
  color: var(--n-text-color);
}

.pane-desc {
  font-size: 14px;
  color: var(--n-text-color-2);
  margin: 0;
  line-height: 1.6;
}

.code-wrapper {
  position: relative;
  border-radius: 12px;
  overflow: hidden;
  background-color: var(--n-code-color);
  border: 1px solid var(--n-border-color);
}

.code-block {
  margin: 0;
  padding: 20px;
  overflow-x: auto;
  font-family: var(--n-font-family-mono);
  font-size: 13.5px;
  line-height: 1.7;
  color: var(--n-text-color-2);
}

.copy-floating {
  position: absolute;
  top: 12px;
  right: 12px;
  opacity: 0.5;
  transition: opacity 0.2s ease, transform 0.2s ease;
  z-index: 10;
}

.code-wrapper:hover .copy-floating {
  opacity: 1;
}

.pane-hint {
  margin-top: 4px;
  padding: 14px 20px;
  border-left: 3px solid var(--n-primary-color);
  background-color: var(--n-action-color);
  border-radius: 0 8px 8px 0;
}

.hint-content {
  font-size: 13px;
  color: var(--n-text-color-3);
  line-height: 1.6;
}

/* For mobile responsiveness */
@media (max-width: 600px) {
  .home-view {
    padding: 20px 12px;
  }
  .home-header {
    margin-bottom: 28px;
  }
  .home-title {
    font-size: 24px;
  }
  .home-desc {
    font-size: 14px;
    margin-bottom: 16px;
  }
  .endpoint-badge {
    flex-direction: column;
    border-radius: 12px;
    align-items: flex-start;
  }
  .section-card {
    padding: 16px;
    border-radius: 12px;
    margin-bottom: 20px;
  }
  .quota-card-top {
    flex-direction: column;
    margin-bottom: 16px;
  }
  .quota-title {
    font-size: 20px;
  }
  .quota-desc {
    font-size: 13px;
  }
  .quota-pill {
    align-self: flex-start;
  }
  .quota-progress-panel {
    padding: 14px;
  }
  .quota-progress-header {
    flex-direction: column;
  }
  .quota-progress-value {
    font-size: 28px;
  }
  .quota-progress-side {
    width: 100%;
    min-width: 0;
  }
  .quota-progress-meta,
  .quota-progress-foot {
    font-size: 12px;
  }
  .guide-tabs :deep(.n-tabs-nav) {
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }
  .guide-tabs :deep(.n-tabs-tab) {
    font-size: 12px;
    padding: 0 8px;
  }
  .pane-title {
    font-size: 17px;
  }
  .pane-desc {
    font-size: 13px;
  }
  .code-block {
    padding: 14px;
    font-size: 12px;
    line-height: 1.6;
  }
  .copy-floating {
    opacity: 0.8;
  }
  .pane-hint {
    padding: 10px 14px;
  }
  .hint-content {
    font-size: 12px;
  }
}

@media (max-width: 520px) {
  .quota-progress-side {
    grid-template-columns: 1fr;
  }
}
</style>
