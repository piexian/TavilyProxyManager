<script setup lang="ts">
import { computed, h } from "vue";
import {
  NButton,
  NDataTable,
  NIcon,
  NPopconfirm,
  NProgress,
  NTag,
  type DataTableColumns,
} from "naive-ui";
import { TrashOutline } from "@vicons/ionicons5";
import { locale, t } from "../../i18n";
import type { KeyItem } from "../../types";
import { useMediaQuery } from "../../composables/useResponsive";
import MobileCardList from "../MobileCardList.vue";
import { TableCell, columnsByKey } from "../TableCell";

const props = defineProps<{
  keys: KeyItem[];
  loading: boolean;
}>();

const emit = defineEmits<{ (e: "delete", id: number): void }>();

// 手机端用卡片列表替代需要横向滚动的表格
const isMobile = useMediaQuery("(max-width: 640px)");

function usagePercent(row: KeyItem): number {
  if (!row.total_quota || row.total_quota <= 0) return 0;
  return Math.max(0, Math.min(100, Math.round((row.used_quota / row.total_quota) * 100)));
}

function statusText(row: KeyItem): string {
  if (row.is_invalid) return t("portal.keys.statusInvalid");
  if (row.is_active) return t("common.active");
  return t("common.disabled");
}

function statusType(row: KeyItem): "success" | "warning" | "error" {
  if (row.is_invalid) return "error";
  return row.is_active ? "success" : "warning";
}

const columns = computed<DataTableColumns<KeyItem>>(() => [
  {
    title: () => t("portal.keys.colAlias"),
    key: "alias",
    width: 150,
    render: (r) => h("span", { style: "font-weight:600" }, r.alias),
  },
  {
    title: () => t("portal.keys.colKey"),
    key: "key",
    render: (r) => h("code", { class: "key-cell" }, r.key),
  },
  {
    title: () => t("portal.keys.colUsage"),
    key: "usage",
    width: 200,
    render: (r) =>
      h("div", { class: "usage-cell" }, [
        h(NProgress, {
          type: "line",
          percentage: usagePercent(r),
          height: 8,
          showIndicator: false,
          borderRadius: 4,
          fillBorderRadius: 4,
          status: usagePercent(r) > 90 ? "error" : "success",
        }),
        h(
          "span",
          { class: "usage-text" },
          `${r.used_quota.toLocaleString(locale.value)} / ${r.total_quota.toLocaleString(locale.value)}`,
        ),
      ]),
  },
  {
    title: () => t("portal.keys.colStatus"),
    key: "status",
    width: 110,
    align: "center",
    render: (r) =>
      h(
        NTag,
        { type: statusType(r), size: "small", round: true, bordered: false },
        { default: () => statusText(r) },
      ),
  },
  {
    title: () => t("portal.keys.colActions"),
    key: "actions",
    width: 80,
    align: "right",
    render: (r) =>
      h(
        NPopconfirm,
        { onPositiveClick: () => emit("delete", r.id) },
        {
          trigger: () =>
            h(
              NButton,
              {
                size: "small",
                quaternary: true,
                type: "error",
                "aria-label": t("portal.keys.deleteConfirm"),
              },
              { icon: () => h(NIcon, { component: TrashOutline }) },
            ),
          default: () => t("portal.keys.deleteConfirm"),
        },
      ),
  },
]);

const col = computed(() => columnsByKey(columns.value));
</script>

<template>
  <MobileCardList
    v-if="isMobile"
    :rows="props.keys"
    :loading="props.loading"
    :row-key="(r: KeyItem) => r.id"
  >
    <template #item="{ row }">
      <div class="mcl-row">
        <div class="mcl-grow"><TableCell :column="col.alias" :row="row" /></div>
        <div class="mcl-actions">
          <TableCell :column="col.status" :row="row" />
          <TableCell :column="col.actions" :row="row" />
        </div>
      </div>
      <TableCell :column="col.key" :row="row" />
      <TableCell :column="col.usage" :row="row" />
    </template>
  </MobileCardList>
  <n-data-table
    v-else
    :columns="columns"
    :data="props.keys"
    :loading="props.loading"
    :row-key="(r: KeyItem) => r.id"
    scroll-x="640"
    size="small"
  />
</template>

<style scoped>
.key-cell {
  background: rgba(14, 165, 233, 0.06);
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
  word-break: break-all;
}

.usage-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
</style>
