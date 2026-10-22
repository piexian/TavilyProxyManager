<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import {
  NCard,
  NIcon,
  NTabPane,
  NTabs,
  useMessage,
} from "naive-ui";
import { PersonCircleOutline } from "@vicons/ionicons5";
import PortalOverview from "../components/portal/PortalOverview.vue";
import PortalAddKeys from "../components/portal/PortalAddKeys.vue";
import PortalKeyTable from "../components/portal/PortalKeyTable.vue";
import PortalSecurity from "../components/portal/PortalSecurity.vue";
import { useDonorPortal } from "../composables/useDonorPortal";
import { t } from "../i18n";
import { apiErrorMessage } from "../utils/apiError";

const props = defineProps<{ refreshNonce?: number }>();

const message = useMessage();
const tab = ref<"overview" | "keys" | "security">("overview");

const {
  me,
  overview,
  timeseries,
  keys,
  loadingKeys,
  loadMe,
  loadOverview,
  loadTimeseries,
  loadKeys,
  deleteKey,
} = useDonorPortal();

// 记住当前所选粒度：重新登录触发的 loadAll 要沿用它，否则图表数据与 PortalOverview
// 里保留的粒度选择不一致（界面显示「天」，实际是小时数据）。
const granularity = ref<"hour" | "day" | "month">("hour");

async function loadAll() {
  await Promise.all([
    loadMe(),
    loadOverview(),
    loadTimeseries(granularity.value),
    loadKeys(),
  ]).catch(() => {
    /* 单项失败不阻断其余；各自的错误在拦截器/界面层处理 */
  });
}

async function onChangeGranularity(g: "hour" | "day" | "month") {
  granularity.value = g;
  await loadTimeseries(g);
}

async function onDelete(id: number) {
  try {
    await deleteKey(id);
    message.success(t("portal.keys.deleted"));
    await Promise.all([loadKeys(), loadOverview()]);
  } catch (err: any) {
    message.error(apiErrorMessage(err, t("common.deleteFailed")));
  }
}

async function onAdded() {
  await Promise.all([loadKeys(), loadOverview()]);
}

async function onReset() {
  await loadMe();
}

watch(
  () => props.refreshNonce,
  (value, previous) => {
    if (value === undefined || previous === undefined || value === previous) return;
    void loadAll();
  },
);

onMounted(loadAll);
</script>

<template>
  <div class="donor-portal">
    <div class="portal-hero">
      <div class="portal-hero__icon">
        <n-icon :component="PersonCircleOutline" />
      </div>
      <div>
        <h1 class="portal-hero__title">{{ t("portal.title") }}</h1>
        <p class="portal-hero__subtitle">
          {{ t("portal.greeting", { alias: me?.alias ?? "" }) }}
        </p>
      </div>
    </div>

    <n-tabs v-model:value="tab" type="line" animated size="large" class="portal-tabs">
      <n-tab-pane name="overview" :tab="t('portal.tabs.overview')">
        <PortalOverview
          :overview="overview"
          :timeseries="timeseries"
          @change-granularity="onChangeGranularity"
        />
      </n-tab-pane>

      <n-tab-pane name="keys" :tab="t('portal.tabs.keys')">
        <div class="keys-pane">
          <n-card size="small" class="section-card">
            <div class="section-title">{{ t("portal.keys.addTitle") }}</div>
            <div class="section-desc">{{ t("portal.keys.addDesc") }}</div>
            <PortalAddKeys @added="onAdded" />
          </n-card>

          <n-card size="small" class="section-card">
            <div class="section-title">{{ t("portal.keys.listTitle") }}</div>
            <PortalKeyTable :keys="keys" :loading="loadingKeys" @delete="onDelete" />
          </n-card>
        </div>
      </n-tab-pane>

      <n-tab-pane name="security" :tab="t('portal.tabs.security')">
        <n-card size="small" class="section-card">
          <div class="section-title">{{ t("portal.security.title") }}</div>
          <div class="section-desc">{{ t("portal.security.desc") }}</div>
          <PortalSecurity :me="me" @reset="onReset" />
        </n-card>
      </n-tab-pane>
    </n-tabs>
  </div>
</template>

<style scoped>
.donor-portal {
  max-width: 1100px;
  margin: 0 auto;
}

.portal-hero {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 20px;
}

.portal-hero__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 52px;
  border-radius: 14px;
  font-size: 30px;
  color: #fff;
  background: #0ea5e9;
  border: 2px solid var(--comic-ink);
  box-shadow: var(--comic-shadow);
}

.portal-hero__title {
  margin: 0;
  font-size: 26px;
  font-weight: 800;
  letter-spacing: -0.03em;
  font-family: var(--comic-font-display);
}

.portal-hero__subtitle {
  margin: 4px 0 0;
  font-size: 14px;
  color: var(--n-text-color-3);
}

.keys-pane {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.section-card {
  border: var(--comic-border) solid var(--comic-ink);
  box-shadow: var(--comic-shadow);
}

.section-title {
  font-size: 18px;
  font-weight: 800;
  letter-spacing: -0.02em;
  margin-bottom: 4px;
}

.section-desc {
  font-size: 13px;
  color: var(--n-text-color-3);
  margin-bottom: 16px;
}

@media (max-width: 640px) {
  .portal-hero__title {
    font-size: 22px;
  }
}
</style>
