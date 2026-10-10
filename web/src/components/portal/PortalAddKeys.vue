<script setup lang="ts">
import { computed, ref } from "vue";
import {
  NButton,
  NInput,
  NRadioButton,
  NRadioGroup,
  NTag,
  useMessage,
} from "naive-ui";
import { api } from "../../api/client";
import { t } from "../../i18n";

const emit = defineEmits<{ (e: "added"): void }>();

const message = useMessage();
const mode = ref<"single" | "batch">("single");
const singleKey = ref("");
const batchKeys = ref("");
const submitting = ref(false);

interface ResultItem {
  key_masked: string;
  alias: string;
  status: string;
}
const resultItems = ref<ResultItem[]>([]);

const isValidKey = (key: string) => key.trim().startsWith("tvly-");

const validBatchKeys = computed(() =>
  batchKeys.value
    .split("\n")
    .map((k) => k.trim())
    .filter((k) => isValidKey(k)),
);

const canSubmit = computed(() =>
  mode.value === "single"
    ? isValidKey(singleKey.value)
    : validBatchKeys.value.length > 0,
);

function statusTagType(status: string): "success" | "info" | "error" | "warning" {
  switch (status) {
    case "created":
      return "success";
    case "duplicated":
      return "info";
    case "invalid_key":
      return "error";
    default:
      return "warning";
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case "created":
      return t("donate.result.statusCreated");
    case "duplicated":
      return t("donate.result.statusDuplicated");
    case "invalid_key":
      return t("donate.result.statusInvalidKey");
    default:
      return t("donate.result.statusInvalid");
  }
}

async function submit() {
  const keys =
    mode.value === "single" ? [singleKey.value.trim()] : validBatchKeys.value;
  if (keys.length === 0) return;

  submitting.value = true;
  try {
    const { data } = await api.post<{
      created: number;
      duplicated: number;
      invalid: number;
      items: ResultItem[];
    }>("/api/portal/keys", { keys });
    resultItems.value = data.items ?? [];
    message.success(
      t("portal.keys.addResult", {
        created: data.created ?? 0,
        duplicated: data.duplicated ?? 0,
        invalid: data.invalid ?? 0,
      }),
    );
    singleKey.value = "";
    batchKeys.value = "";
    emit("added");
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("common.createFailed"));
  } finally {
    submitting.value = false;
  }
}
</script>

<template>
  <div class="portal-add">
    <div class="add-row">
      <n-radio-group v-model:value="mode" size="small">
        <n-radio-button value="single">{{ t("donate.single") }}</n-radio-button>
        <n-radio-button value="batch">{{ t("donate.batch") }}</n-radio-button>
      </n-radio-group>
    </div>

    <div v-if="mode === 'single'" class="single-row">
      <n-input
        v-model:value="singleKey"
        :placeholder="t('donate.keyPlaceholder')"
        :status="singleKey && !isValidKey(singleKey) ? 'error' : undefined"
        @keyup.enter="submit"
      />
      <n-button
        type="primary"
        :loading="submitting"
        :disabled="!canSubmit"
        @click="submit"
      >
        {{ t("portal.keys.addButton") }}
      </n-button>
    </div>

    <div v-else class="batch-area">
      <n-input
        v-model:value="batchKeys"
        type="textarea"
        :placeholder="t('donate.batchPlaceholder')"
        :autosize="{ minRows: 4, maxRows: 9 }"
      />
      <div class="batch-footer">
        <span class="count-info">
          {{ t("donate.validKeys", { count: validBatchKeys.length }) }}
        </span>
        <n-button
          type="primary"
          :loading="submitting"
          :disabled="!canSubmit"
          @click="submit"
        >
          {{ t("portal.keys.addButton") }}
        </n-button>
      </div>
    </div>

    <div v-if="resultItems.length" class="result-list">
      <div v-for="(item, idx) in resultItems" :key="idx" class="result-row">
        <code class="mono">{{ item.key_masked }}</code>
        <n-tag :type="statusTagType(item.status)" size="small" round>
          {{ statusLabel(item.status) }}
        </n-tag>
      </div>
    </div>
  </div>
</template>

<style scoped>
.portal-add {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.single-row {
  display: flex;
  gap: 12px;
}

.batch-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 10px;
}

.count-info {
  font-size: 13px;
  color: var(--n-text-color-3);
}

.result-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px;
  border: 2px solid var(--comic-ink);
  border-radius: 10px;
  background: rgba(14, 165, 233, 0.05);
}

.result-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
}

.mono {
  font-family: var(--n-font-family-mono);
  word-break: break-all;
}

@media (max-width: 600px) {
  .single-row {
    flex-direction: column;
  }
}
</style>
