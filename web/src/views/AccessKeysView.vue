<template>
  <n-space vertical size="large">
    <div class="page-header">
      <div class="header-info">
        <h2 class="page-title">{{ t("accessKeys.title") }}</h2>
        <div class="page-subtitle">
          {{ t("accessKeys.subtitle") }}
        </div>
      </div>
      <n-space align="center">
        <n-button type="primary" @click="showGenerateModal = true">
          <template #icon>
            <n-icon :component="AddOutline" />
          </template>
          {{ t("accessKeys.generate") }}
        </n-button>
      </n-space>
    </div>

    <n-card :bordered="false" class="table-card">
      <n-data-table
        :columns="columns"
        :data="items"
        :loading="loading"
        :row-key="(r: AccessKeyItem) => r.id"
        scroll-x="920"
        size="small"
      />
    </n-card>

    <n-modal
      v-model:show="showGenerateModal"
      preset="dialog"
      :title="t('accessKeys.generateModal.title')"
      :positive-text="t('accessKeys.generateModal.generate')"
      :negative-text="t('common.cancel')"
      :loading="generating"
      @positive-click="generateKey"
    >
      <n-form-item :label="t('accessKeys.generateModal.alias')">
        <n-input
          v-model:value="newAlias"
          :placeholder="t('accessKeys.generateModal.aliasPlaceholder')"
        />
      </n-form-item>
    </n-modal>

    <n-modal
      v-model:show="showGeneratedModal"
      preset="card"
      :title="t('accessKeys.generatedModal.title')"
      style="max-width: 560px"
      :mask-closable="false"
    >
      <n-space vertical size="large">
        <n-alert type="warning" :show-icon="true">
          {{ t("accessKeys.generatedModal.warning") }}
        </n-alert>
        <div>
          <div class="field-label">{{ t("accessKeys.generatedModal.key") }}</div>
          <n-input-group>
            <n-input :value="generatedKey" readonly />
            <n-button type="primary" ghost @click="copyGenerated">
              <template #icon><n-icon :component="CopyOutline" /></template>
            </n-button>
          </n-input-group>
        </div>
      </n-space>
      <template #footer>
        <n-space justify="end">
          <n-button type="primary" @click="showGeneratedModal = false">
            {{ t("common.dismiss") }}
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal
      v-model:show="showEditModal"
      preset="dialog"
      :title="t('accessKeys.editModal.title')"
      :positive-text="t('accessKeys.editModal.save')"
      :negative-text="t('common.cancel')"
      :loading="saving"
      @positive-click="saveEdit"
    >
      <n-space vertical size="medium">
        <n-form-item :label="t('accessKeys.editModal.alias')">
          <n-input v-model:value="editAlias" />
        </n-form-item>
        <n-form-item :label="t('accessKeys.editModal.status')">
          <n-space align="center">
            <n-switch v-model:value="editActive" />
            <span>{{ editActive ? t("common.enabled") : t("common.disabled") }}</span>
          </n-space>
        </n-form-item>
      </n-space>
    </n-modal>
  </n-space>
</template>

<script setup lang="ts">
import { h, onMounted, ref } from "vue";
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NFormItem,
  NIcon,
  NInput,
  NInputGroup,
  NModal,
  NPopconfirm,
  NSpace,
  NSwitch,
  NTag,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import {
  AddOutline,
  CopyOutline,
  CreateOutline,
  TrashOutline,
} from "@vicons/ionicons5";
import { api } from "../api/client";
import type { AccessKeyItem } from "../types";
import { writeClipboardText } from "../utils/clipboard";
import { locale, t } from "../i18n";

const message = useMessage();
const items = ref<AccessKeyItem[]>([]);
const loading = ref(false);

const showGenerateModal = ref(false);
const generating = ref(false);
const newAlias = ref("");

const showGeneratedModal = ref(false);
const generatedKey = ref("");

const showEditModal = ref(false);
const saving = ref(false);
const editId = ref(0);
const editAlias = ref("");
const editActive = ref(true);

function formatTime(value?: string | null): string {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString(locale.value);
}

async function loadKeys() {
  loading.value = true;
  try {
    const { data } = await api.get<{ items: AccessKeyItem[] }>("/api/access-keys");
    items.value = data.items;
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("accessKeys.errors.load"));
  } finally {
    loading.value = false;
  }
}

async function generateKey() {
  const alias = newAlias.value.trim();
  if (!alias) {
    message.warning(t("accessKeys.errors.missingAlias"));
    return false;
  }
  generating.value = true;
  try {
    const { data } = await api.post<{ item: AccessKeyItem & { key: string } }>(
      "/api/access-keys",
      { alias },
    );
    generatedKey.value = data.item.key;
    showGenerateModal.value = false;
    showGeneratedModal.value = true;
    newAlias.value = "";
    message.success(t("accessKeys.messages.generated"));
    await loadKeys();
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("common.createFailed"));
  } finally {
    generating.value = false;
  }
  return false;
}

async function copyGenerated() {
  try {
    await writeClipboardText(generatedKey.value);
    message.success(t("common.copiedToClipboard"));
  } catch {
    message.error(t("common.copyFailed"));
  }
}

async function copyRawKey(id: number) {
  try {
    const { data } = await api.get<{ key: string }>(`/api/access-keys/${id}/raw`);
    await writeClipboardText(data.key);
    message.success(t("common.copiedToClipboard"));
  } catch {
    message.error(t("common.copyFailed"));
  }
}

function openEdit(row: AccessKeyItem) {
  editId.value = row.id;
  editAlias.value = row.alias;
  editActive.value = row.is_active;
  showEditModal.value = true;
}

async function saveEdit() {
  saving.value = true;
  try {
    await api.put(`/api/access-keys/${editId.value}`, {
      alias: editAlias.value,
      is_active: editActive.value,
    });
    showEditModal.value = false;
    message.success(t("accessKeys.messages.updated"));
    await loadKeys();
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("common.updateFailed"));
  } finally {
    saving.value = false;
  }
  return false;
}

async function deleteKey(id: number) {
  try {
    await api.delete(`/api/access-keys/${id}`);
    message.success(t("accessKeys.messages.deleted"));
    await loadKeys();
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("common.deleteFailed"));
  }
}

const columns: DataTableColumns<AccessKeyItem> = [
  {
    title: () => t("accessKeys.table.alias"),
    key: "alias",
    width: 180,
    render: (r) => h("span", { style: "font-weight: 600" }, r.alias),
  },
  {
    title: () => t("accessKeys.table.key"),
    key: "key",
    render: (r) => h("code", { class: "key-cell" }, r.key),
  },
  {
    title: () => t("accessKeys.table.status"),
    key: "is_active",
    width: 100,
    align: "center",
    render: (r) =>
      h(
        NTag,
        {
          type: r.is_active ? "success" : "default",
          size: "small",
          round: true,
          bordered: false,
        },
        { default: () => (r.is_active ? t("common.active") : t("common.disabled")) },
      ),
  },
  {
    title: () => t("accessKeys.table.lastUsed"),
    key: "last_used_at",
    width: 180,
    render: (r) =>
      h("span", { class: "time-cell" }, formatTime(r.last_used_at)),
  },
  {
    title: () => t("accessKeys.table.created"),
    key: "created_at",
    width: 180,
    render: (r) =>
      h("span", { class: "time-cell" }, formatTime(r.created_at)),
  },
  {
    title: () => t("accessKeys.table.actions"),
    key: "actions",
    width: 160,
    align: "right",
    render: (r) =>
      h(NSpace, { size: 4, justify: "end" }, () => [
        h(
          NButton,
          {
            size: "small",
            quaternary: true,
            onClick: () => copyRawKey(r.id),
          },
          {
            icon: () => h(NIcon, { component: CopyOutline }),
          },
        ),
        h(
          NButton,
          {
            size: "small",
            quaternary: true,
            onClick: () => openEdit(r),
          },
          {
            icon: () => h(NIcon, { component: CreateOutline }),
          },
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => deleteKey(r.id) },
          {
            trigger: () =>
              h(
                NButton,
                { size: "small", quaternary: true, type: "error" },
                { icon: () => h(NIcon, { component: TrashOutline }) },
              ),
            default: () => t("accessKeys.confirm.delete"),
          },
        ),
      ]),
  },
];

onMounted(loadKeys);
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

.field-label {
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 8px;
  color: #666;
}

.key-cell {
  background: rgba(0, 0, 0, 0.03);
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
}

.time-cell {
  color: #888;
  font-size: 13px;
}

@media (max-width: 640px) {
  .field-label {
    font-size: 13px;
  }

  .key-cell {
    font-size: 11px;
    word-break: break-all;
  }
}
</style>
