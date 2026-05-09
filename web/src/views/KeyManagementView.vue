<template>
  <n-space vertical size="large">
    <div class="page-header">
      <div class="header-info">
        <h2 class="page-title">{{ t("keys.title") }}</h2>
        <div class="page-subtitle">
          {{ t("keys.subtitle") }}
        </div>
      </div>
      <n-space align="center" :size="[12, 12]">
        <n-tooltip>
          <template #trigger>
            <n-input-number
              v-model:value="syncIntervalSeconds"
              :min="0"
              :max="60"
              :disabled="syncAllBusy"
              size="medium"
              style="width: 120px"
            >
              <template #suffix>{{ t("keys.suffix.seconds") }}</template>
            </n-input-number>
          </template>
          {{ t("keys.tooltip.delayBetweenUsage") }}
        </n-tooltip>
        <n-button :loading="syncAllBusy" :disabled="syncAllBusy" @click="syncAll" secondary>
          <template #icon>
            <n-icon :component="SyncOutline" />
          </template>
          {{ t("keys.syncAll") }}
        </n-button>
        <n-popconfirm
          :disabled="invalidCount === 0 || syncAllBusy || deletingInvalid"
          @positive-click="deleteInvalidKeys"
        >
          <template #trigger>
            <n-button
              type="error"
              secondary
              :loading="deletingInvalid"
              :disabled="invalidCount === 0 || syncAllBusy"
            >
              <template #icon>
                <n-icon :component="TrashOutline" />
              </template>
              {{ t("keys.actions.deleteInvalid") }} ({{ invalidCount }})
            </n-button>
          </template>
          {{ t("keys.confirm.deleteInvalid", { count: invalidCount }) }}
        </n-popconfirm>
        <n-button :disabled="syncAllBusy" @click="exportKeys" secondary>
          <template #icon>
            <n-icon :component="DownloadOutline" />
          </template>
          {{ t("keys.exportAll") }}
        </n-button>
        <n-button secondary @click="openBatchAdd">
          <template #icon>
            <n-icon :component="ListOutline" />
          </template>
          {{ t("keys.batchAdd") }}
        </n-button>
        <n-button type="primary" @click="showAdd = true">
          <template #icon>
            <n-icon :component="AddOutline" />
          </template>
          {{ t("keys.addNewKey") }}
        </n-button>
      </n-space>
    </div>

    <n-card v-if="syncJob.status === 'running'" :bordered="false" size="small">
      <n-space vertical size="small">
        <div class="sync-job-title">
          {{
            t("keys.messages.syncAllProgress", {
              completed: syncJob.completed ?? 0,
              total: syncJob.total ?? 0,
              failed: syncJob.failed ?? 0,
            })
          }}
        </div>
        <n-progress
          type="line"
          :percentage="syncJobPercent"
          :show-indicator="false"
          :height="8"
        />
      </n-space>
    </n-card>

    <n-card :bordered="false" class="table-card">
      <n-data-table
        :columns="columns"
        :data="items"
        :loading="loading"
        :row-key="rowKey"
        :pagination="pagination"
        scroll-x="1150"
        size="small"
      />
    </n-card>

    <n-modal
      v-model:show="showAdd"
      preset="card"
      :title="t('keys.addModal.title')"
      style="max-width: 560px"
      class="custom-modal"
    >
      <n-form :model="addForm" label-placement="top" size="large">
        <n-form-item :label="t('keys.addModal.apiKey')">
          <n-input v-model:value="addForm.key" placeholder="tvly-..." />
        </n-form-item>
        <n-form-item :label="t('keys.addModal.alias')">
          <n-input
            v-model:value="addForm.alias"
            :placeholder="t('keys.addModal.aliasPlaceholder')"
          />
        </n-form-item>
        <n-form-item :label="t('keys.addModal.totalQuota')">
          <n-input-number
            v-model:value="addForm.total_quota"
            :min="1"
            style="width: 100%"
          />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showAdd = false">{{ t("common.cancel") }}</n-button>
          <n-button type="primary" :loading="saving" @click="createKey"
            >{{ t("keys.addModal.createKey") }}</n-button
          >
        </n-space>
      </template>
    </n-modal>

    <n-modal
      v-model:show="showBatchAdd"
      preset="card"
      :title="t('keys.batchModal.title')"
      style="max-width: 560px"
      class="custom-modal"
    >
      <n-space vertical size="large">
        <n-alert type="info" :show-icon="false" size="small">
          {{ t("keys.batchModal.help") }}
        </n-alert>
        <n-input
          v-model:value="batchText"
          type="textarea"
          :placeholder="'alias----tvly-...\ntvly-...'"
          :autosize="{ minRows: 6, maxRows: 12 }"
        />
        <n-alert
          v-if="batchFailures.length"
          type="error"
          closable
          class="batch-error"
        >
          <div>
            {{ t("keys.batchModal.failedHeader", { count: batchFailures.length }) }}
          </div>
          <div class="batch-error-list">
            <div
              v-for="item in batchFailures.slice(0, 10)"
              :key="item.key"
              class="batch-error-item"
            >
              {{ item.key }} — {{ item.error }}
            </div>
            <div v-if="batchFailures.length > 10" class="batch-error-more">
              {{ t("common.andMore", { count: batchFailures.length - 10 }) }}
            </div>
          </div>
        </n-alert>
      </n-space>
      <template #footer>
        <n-space justify="end">
          <n-button :disabled="batchSaving" @click="closeBatchAdd"
            >{{ t("common.cancel") }}</n-button
          >
          <n-button
            type="primary"
            :loading="batchSaving"
            :disabled="!batchText.trim()"
            @click="createBatchKeys"
            >{{ t("keys.batchModal.addKeys") }}</n-button
          >
        </n-space>
      </template>
    </n-modal>

    <n-modal
      v-model:show="showEdit"
      preset="card"
      :title="t('keys.editModal.title')"
      style="max-width: 560px"
      class="custom-modal"
    >
      <n-form :model="editForm" label-placement="top" size="large">
        <n-form-item :label="t('keys.editModal.alias')">
          <n-input v-model:value="editForm.alias" />
        </n-form-item>
        <n-grid cols="1 s:2" responsive="screen" :x-gap="12">
          <n-form-item-gi :label="t('keys.editModal.totalQuota')">
            <n-input-number
              v-model:value="editForm.total_quota"
              :min="1"
              style="width: 100%"
            />
          </n-form-item-gi>
          <n-form-item-gi :label="t('keys.editModal.usedQuota')">
            <n-input-number
              v-model:value="editForm.used_quota"
              :min="0"
              style="width: 100%"
            />
          </n-form-item-gi>
        </n-grid>
        <n-form-item :label="t('keys.editModal.status')">
          <n-space align="center">
            <n-switch v-model:value="editForm.is_active" :disabled="editIsInvalid" />
            <span>{{
              editIsInvalid
                ? t("keys.status.invalid")
                : editForm.is_active
                  ? t("common.active")
                  : t("common.disabled")
            }}</span>
          </n-space>
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showEdit = false">{{ t("common.cancel") }}</n-button>
          <n-button type="primary" :loading="saving" @click="saveEdit"
            >{{ t("keys.editModal.saveChanges") }}</n-button
          >
        </n-space>
      </template>
    </n-modal>
  </n-space>
</template>

<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NForm,
  NFormItem,
  NFormItemGi,
  NGrid,
  NIcon,
  NInput,
  NInputNumber,
  NModal,
  NPopconfirm,
  NProgress,
  NSpace,
  NSwitch,
  NTag,
  NTooltip,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import {
  AddOutline,
  CopyOutline,
  CreateOutline,
  DownloadOutline,
  ListOutline,
  RefreshOutline,
  SyncOutline,
  TrashOutline,
} from "@vicons/ionicons5";
import { api } from "../api/client";
import type { KeyItem } from "../types";
import { writeClipboardText } from "../utils/clipboard";
import { t } from "../i18n";

const message = useMessage();
const items = ref<KeyItem[]>([]);
const loading = ref(false);
const saving = ref(false);
const syncStarting = ref(false);
const deletingInvalid = ref(false);
const syncingUsageIds = ref(new Set<number>());
const invalidCount = computed(
  () => items.value.filter((item) => item.is_invalid).length
);

type SyncJob = {
  id?: string;
  status: "idle" | "running" | "completed" | "error";
  error?: string;
  interval_ms?: number;
  total?: number;
  completed?: number;
  succeeded?: number;
  failed?: number;
  started_at?: string;
  ended_at?: string;
};

const syncJob = ref<SyncJob>({ status: "idle" });
const syncingAll = computed(() => syncJob.value.status === "running");
const syncAllBusy = computed(() => syncStarting.value || syncingAll.value);
const syncJobPercent = computed(() => {
  const total = syncJob.value.total ?? 0;
  const completed = syncJob.value.completed ?? 0;
  if (total <= 0) return 0;
  return Math.min(100, Math.max(0, Math.floor((completed / total) * 100)));
});

let syncJobPoll: number | null = null;

function startSyncJobPolling(): void {
  if (syncJobPoll != null) return;
  syncJobPoll = window.setInterval(() => {
    void loadSyncJob();
  }, 1000);
}

function stopSyncJobPolling(): void {
  if (syncJobPoll == null) return;
  window.clearInterval(syncJobPoll);
  syncJobPoll = null;
}

const SYNC_INTERVAL_SECONDS_STORAGE_KEY = "tavily_proxy_sync_interval_seconds";
function normalizeIntervalSeconds(value: unknown): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed < 0) return 0;
  return Math.min(60, Math.floor(parsed));
}

const syncIntervalSeconds = ref<number>(
  normalizeIntervalSeconds(localStorage.getItem(SYNC_INTERVAL_SECONDS_STORAGE_KEY))
);

watch(syncIntervalSeconds, (value) => {
  localStorage.setItem(
    SYNC_INTERVAL_SECONDS_STORAGE_KEY,
    String(normalizeIntervalSeconds(value))
  );
});

const showAdd = ref(false);
const addForm = reactive<{ key: string; alias: string; total_quota: number }>({
  key: "",
  alias: "",
  total_quota: 1000,
});

const showBatchAdd = ref(false);
const batchText = ref("");
const batchSaving = ref(false);
const batchFailures = ref<{ key: string; error: string }[]>([]);

const showEdit = ref(false);
const editId = ref<number | null>(null);
const editIsInvalid = ref(false);
const editForm = reactive<{
  alias: string;
  total_quota: number;
  used_quota: number;
  is_active: boolean;
}>({
  alias: "",
  total_quota: 1000,
  used_quota: 0,
  is_active: true,
});

const pagination = reactive({
  pageSize: 10,
});

function rowKey(row: KeyItem) {
  return row.id;
}

async function refresh() {
  loading.value = true;
  try {
    const { data } = await api.get<{ items: KeyItem[] }>("/api/keys");
    items.value = data.items;
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("keys.errors.loadKeys"));
  } finally {
    loading.value = false;
  }
}

async function loadSyncJob(): Promise<void> {
  try {
    const prev = syncJob.value;
    const { data } = await api.get<SyncJob>("/api/keys/sync");
    syncJob.value = data;

    if (data.status === "running") {
      startSyncJobPolling();
      return;
    }

    stopSyncJobPolling();

    const finishedSameJob =
      prev.status === "running" && prev.id && prev.id === data.id;

    if (!finishedSameJob) return;

    await refresh();

    if (data.status === "completed") {
      message.success(
        t("keys.messages.syncAllSuccess", {
          succeeded: data.succeeded ?? 0,
          total: data.total ?? 0,
          failed: data.failed ?? 0,
        })
      );
      return;
    }

    message.error(data.error ?? t("keys.errors.syncAllFailed"));
  } catch (err: any) {
    stopSyncJobPolling();
    message.error(err?.response?.data?.error ?? t("keys.errors.syncAllFailed"));
  }
}

async function syncAll() {
  if (syncAllBusy.value) return;
  syncStarting.value = true;
  try {
    const intervalSeconds = normalizeIntervalSeconds(syncIntervalSeconds.value);
    const { data } = await api.post<SyncJob>("/api/keys/sync", {
      interval_ms: intervalSeconds * 1000,
    });
    syncJob.value = data;
    message.success(t("keys.messages.syncAllStarted"));
    startSyncJobPolling();
    void loadSyncJob();
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("keys.errors.syncAllFailed"));
  } finally {
    syncStarting.value = false;
  }
}

async function createKey() {
  saving.value = true;
  try {
    await api.post("/api/keys", addForm);
    showAdd.value = false;
    addForm.key = "";
    addForm.alias = "";
    addForm.total_quota = 1000;
    await refresh();
    message.success(t("keys.messages.keyAdded"));
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("common.createFailed"));
  } finally {
    saving.value = false;
  }
}

function openBatchAdd() {
  batchText.value = "";
  batchFailures.value = [];
  showBatchAdd.value = true;
}

function closeBatchAdd() {
  if (batchSaving.value) return;
  showBatchAdd.value = false;
  batchText.value = "";
  batchFailures.value = [];
}

function parseBatchKeys(input: string): { key: string; alias: string }[] {
  const out: { key: string; alias: string }[] = [];
  const seen = new Set<string>();
  for (const raw of input.split(/\r?\n/)) {
    let line = raw.trim();
    if (!line) continue;

    let key = "";
    let alias = "";

    const separators = ["----"];
    for (const sep of separators) {
      if (line.includes(sep)) {
        const parts = line.split(sep);
        const lastPart = parts[parts.length - 1]?.trim();
        if (lastPart && lastPart.startsWith("tvly-")) {
          key = lastPart;
          alias = parts[0]?.trim() || "";
          break;
        }
      }
    }

    if (!key) {
      key = line;
    }

    if (!key.startsWith("tvly-")) continue;

    if (seen.has(key)) continue;
    seen.add(key);
    out.push({ key, alias });
  }
  return out;
}

async function createBatchKeys() {
  const entries = parseBatchKeys(batchText.value);
  if (entries.length === 0) {
    message.error(t("keys.errors.needAtLeastOneKey"));
    return;
  }

  batchSaving.value = true;
  batchFailures.value = [];

  let succeeded = 0;
  for (const entry of entries) {
    try {
      const payload: Record<string, string> = { key: entry.key };
      if (entry.alias) payload.alias = entry.alias;
      await api.post("/api/keys", payload);
      succeeded++;
    } catch (err: any) {
      batchFailures.value.push({
        key: entry.alias ? `${entry.alias} (${entry.key})` : entry.key,
        error: err?.response?.data?.error ?? t("common.createFailed"),
      });
    }
  }

  batchSaving.value = false;
  await refresh();

  if (batchFailures.value.length === 0) {
    showBatchAdd.value = false;
    batchText.value = "";
    message.success(t("keys.messages.addedKeys", { count: succeeded }));
    return;
  }

  message.warning(
    t("keys.messages.addedPartial", {
      succeeded,
      total: entries.length,
      failed: batchFailures.value.length,
    })
  );
}

function openEdit(row: KeyItem) {
  editId.value = row.id;
  editIsInvalid.value = row.is_invalid;
  editForm.alias = row.alias;
  editForm.total_quota = row.total_quota;
  editForm.used_quota = row.used_quota;
  editForm.is_active = row.is_active;
  showEdit.value = true;
}

async function saveEdit() {
  if (editId.value == null) return;
  saving.value = true;
  try {
    await api.put(`/api/keys/${editId.value}`, {
      alias: editForm.alias,
      total_quota: editForm.total_quota,
      used_quota: editForm.used_quota,
      is_active: editForm.is_active,
    });
    showEdit.value = false;
    await refresh();
    message.success(t("keys.messages.updated"));
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("common.updateFailed"));
  } finally {
    saving.value = false;
  }
}

async function toggleActive(row: KeyItem, value: boolean) {
  if (row.is_invalid) return;
  try {
    await api.put(`/api/keys/${row.id}`, { is_active: value });
    row.is_active = value;
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("common.updateFailed"));
  }
}

async function resetQuota(row: KeyItem) {
  try {
    await api.put(`/api/keys/${row.id}`, { reset_quota: true });
    await refresh();
    message.success(t("keys.messages.quotaReset"));
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("common.resetFailed"));
  }
}

async function syncUsage(row: KeyItem) {
  if (row.is_invalid || syncingUsageIds.value.has(row.id)) return;
  syncingUsageIds.value.add(row.id);
  try {
    await api.put(`/api/keys/${row.id}`, { sync_usage: true });
    message.success(t("keys.messages.syncedFromUsage"));
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("common.syncFailed"));
  } finally {
    await refresh();
    syncingUsageIds.value.delete(row.id);
  }
}

async function deleteInvalidKeys() {
  if (invalidCount.value === 0) return;
  deletingInvalid.value = true;
  try {
    const { data } = await api.delete<{ deleted: number }>("/api/keys/invalid");
    await refresh();
    message.success(t("keys.messages.deletedInvalid", { count: data.deleted }));
  } catch (err: any) {
    message.error(
      err?.response?.data?.error ?? t("keys.errors.deleteInvalidFailed")
    );
  } finally {
    deletingInvalid.value = false;
  }
}

async function deleteKey(row: KeyItem) {
  try {
    await api.delete(`/api/keys/${row.id}`);
    await refresh();
    message.success(t("keys.messages.deleted"));
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("common.deleteFailed"));
  }
}

async function copyToClipboard(row: KeyItem) {
  try {
    const { data } = await api.get<{ key: string }>(`/api/keys/${row.id}/raw`);
    await writeClipboardText(data.key);
    message.success(t("common.copiedToClipboard"));
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("common.copyFailed"));
  }
}

async function exportKeys() {
  try {
    const resp = await api.get("/api/keys/export", { responseType: "blob" });
    const blob =
      resp.data instanceof Blob
        ? resp.data
        : new Blob([resp.data], { type: "text/plain; charset=utf-8" });

    const date = new Date();
    const pad = (n: number) => String(n).padStart(2, "0");
    const filename = `tavily-keys-${date.getFullYear()}${pad(
      date.getMonth() + 1
    )}${pad(date.getDate())}.txt`;

    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);

    const exportedCount = Number(resp.headers?.["x-exported-count"] ?? 0);
    message.success(
      t("keys.messages.exported", {
        count: Number.isFinite(exportedCount) ? exportedCount : 0,
      })
    );
  } catch {
    message.error(t("keys.errors.exportFailed"));
  }
}

const columns: DataTableColumns<KeyItem> = [
  {
    title: () => t("keys.table.alias"),
    key: "alias",
    width: 150,
    render: (row) =>
      h("div", { class: "alias-cell" }, [
        row.alias,
        ...(row.is_donated
          ? [
              h(
                NTag,
                { size: "tiny", type: "info", round: true, bordered: false, style: "margin-left: 6px" },
                { default: () => t("keys.status.donated") }
              ),
            ]
          : []),
      ]),
  },
  {
    title: () => t("keys.table.key"),
    key: "key",
    render: (row) =>
      h(
        NSpace,
        { align: "center", size: "small" },
        {
          default: () => [
            h("code", { class: "key-code" }, row.key),
            h(
              NButton,
              {
                size: "tiny",
                quaternary: true,
                circle: true,
                onClick: () => copyToClipboard(row),
              },
              { icon: () => h(NIcon, { component: CopyOutline }) }
            ),
          ],
        }
      ),
  },
  {
    title: () => t("keys.table.usageQuota"),
    key: "quota",
    width: 250,
    render: (row) => {
      const total = row.total_quota || 0;
      const used = row.used_quota || 0;
      const pct =
        total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0;
      return h(
        NSpace,
        { vertical: true, size: 4, style: "width: 100%" },
        {
          default: () => [
            h("div", { class: "quota-info" }, [
              h("span", { class: "quota-text" }, `${used} / ${total}`),
              h(
                NTag,
                {
                  size: "tiny",
                  round: true,
                  bordered: false,
                  type: pct >= 90 ? "error" : pct >= 70 ? "warning" : "success",
                },
                { default: () => `${pct}%` }
              ),
            ]),
            h(NProgress, {
              type: "line",
              percentage: pct,
              showIndicator: false,
              status: pct >= 90 ? "error" : pct >= 70 ? "warning" : "success",
              height: 6,
            }),
          ],
        }
      );
    },
  },
  {
    title: () => t("keys.table.status"),
    key: "is_active",
    width: 100,
    align: "center",
    render: (row) =>
      row.is_invalid
        ? h(
            NTag,
            { size: "small", type: "error" },
            { default: () => t("keys.status.invalid") }
          )
        : h(NSwitch, {
            size: "small",
            value: row.is_active,
            onUpdateValue: (v: boolean) => toggleActive(row, v),
          }),
  },
  {
    title: () => t("keys.table.actions"),
    key: "actions",
    width: 200,
    align: "right",
    render: (row) =>
      h(
        NSpace,
        { size: "small", justify: "end" },
        {
          default: () => [
            h(
              NTooltip,
              {},
              {
                trigger: () =>
                  h(
                    NButton,
                    {
                      size: "small",
                      quaternary: true,
                      circle: true,
                      disabled: row.is_invalid,
                      loading: syncingUsageIds.value.has(row.id),
                      onClick: () => syncUsage(row),
                    },
                    {
                      icon: () => h(NIcon, { component: SyncOutline }),
                    }
                  ),
                default: () => t("keys.actions.syncUsage"),
              }
            ),
            h(
              NTooltip,
              {},
              {
                trigger: () =>
                  h(
                    NButton,
                    {
                      size: "small",
                      quaternary: true,
                      circle: true,
                      onClick: () => resetQuota(row),
                    },
                    {
                      icon: () => h(NIcon, { component: RefreshOutline }),
                    }
                  ),
                default: () => t("keys.actions.resetQuota"),
              }
            ),
            h(
              NTooltip,
              {},
              {
                trigger: () =>
                  h(
                    NButton,
                    {
                      size: "small",
                      quaternary: true,
                      circle: true,
                      onClick: () => openEdit(row),
                    },
                    {
                      icon: () => h(NIcon, { component: CreateOutline }),
                    }
                  ),
                default: () => t("keys.actions.editKey"),
              }
            ),
            h(
              NPopconfirm,
              { onPositiveClick: () => deleteKey(row) },
              {
                trigger: () =>
                  h(
                    NTooltip,
                    {},
                    {
                      trigger: () =>
                        h(
                          NButton,
                          {
                            size: "small",
                            quaternary: true,
                            circle: true,
                            type: "error",
                          },
                          {
                            icon: () => h(NIcon, { component: TrashOutline }),
                          }
                        ),
                      default: () => t("keys.actions.deleteKey"),
                    }
                  ),
                default: () => t("keys.confirm.deleteKey"),
              }
            ),
          ],
        }
      ),
  },
];

onMounted(async () => {
  await refresh();
  await loadSyncJob();
});

onBeforeUnmount(stopSyncJobPolling);
</script>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.header-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.page-title {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
}

.page-subtitle {
  color: #888;
  font-size: 13px;
}

.table-card {
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
}

.table-card :deep(.n-card__content) {
  padding: 0;
}

.alias-cell {
  font-weight: 600;
}

.key-code {
  background: rgba(0, 0, 0, 0.05);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: monospace;
  font-size: 13px;
}

.quota-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.quota-text {
  font-size: 13px;
  color: #666;
  font-family: monospace;
}

.custom-modal {
  border-radius: 16px;
}

.batch-error {
  border-radius: 8px;
}

.batch-error-list {
  margin-top: 8px;
  max-height: 140px;
  overflow: auto;
}

.batch-error-item {
  font-family: monospace;
  font-size: 12px;
  line-height: 1.4;
}

.batch-error-more {
  margin-top: 6px;
  font-size: 12px;
  opacity: 0.8;
}

:deep(.n-data-table-td) {
  padding: 12px 16px;
}

@media (max-width: 640px) {
  .custom-modal {
    width: calc(100vw - 24px);
  }

  .quota-info {
    gap: 8px;
  }

  .quota-text {
    font-size: 12px;
  }

  :deep(.n-data-table-th),
  :deep(.n-data-table-td) {
    padding: 10px 12px;
  }
}
</style>
