<template>
  <div class="key-donation">
    <div class="donation-header">
      <div class="header-icon">&#128273;</div>
      <div class="header-text">
        <h2 class="title">{{ t("donate.title") }}</h2>
        <p class="desc">{{ t("donate.description") }}</p>
      </div>
    </div>

    <n-tabs type="segment" animated v-model:value="activeTab">
      <n-tab-pane name="donate" :tab="t('donate.tabDonate')">
        <div class="tab-content">
          <div class="form-item">
            <label class="item-label" for="donate-alias">
              {{ t("donate.aliasLabel") }} <span class="required">*</span>
            </label>
            <n-input
              id="donate-alias"
              v-model:value="alias"
              :placeholder="t('donate.aliasPlaceholder')"
              maxlength="50"
              show-count
            />
          </div>

          <div class="form-item">
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
              @keypress.enter="handleDonate"
            />
            <n-button
              type="primary"
              :loading="submitting"
              :disabled="!canSubmitSingle"
              @click="handleDonate"
            >
              {{ t("donate.submit") }}
            </n-button>
          </div>

          <div v-else class="batch-area">
            <n-input
              v-model:value="batchKeys"
              type="textarea"
              :placeholder="t('donate.batchPlaceholder')"
              :autosize="{ minRows: 5, maxRows: 10 }"
            />
            <div class="batch-footer">
              <div class="count-info" :class="{ 'has-invalid': hasInvalidInBatch }">
                {{ t("donate.validKeys", { count: validBatchKeys.length }) }}
              </div>
              <n-button
                type="primary"
                :loading="submitting"
                :disabled="!canSubmitBatch"
                @click="handleDonate"
              >
                {{ t("donate.submit") }}
              </n-button>
            </div>
          </div>

          <div class="donate-hint">
            <div class="hint-content">{{ t("donate.hint") }}</div>
          </div>
        </div>
      </n-tab-pane>

      <n-tab-pane name="query" :tab="t('donate.tabQuery')">
        <div class="tab-content">
          <div class="single-row">
            <n-input
              v-model:value="queryKey"
              :placeholder="t('donate.query.keyPlaceholder')"
              @keypress.enter="handleQuery"
            />
            <n-button type="primary" :loading="querying" @click="handleQuery">
              {{ t("donate.query.submit") }}
            </n-button>
          </div>

          <div v-if="queryResult" class="query-result-wrapper">
            <div v-if="queryResult.found" class="result-found">
              <div class="result-item">
                <span class="label">{{ t("donate.query.alias") }}:</span>
                <span class="value">{{ queryResult.alias }}</span>
              </div>
              <div class="result-item">
                <span class="label">{{ t("donate.query.status") }}:</span>
                <n-tag :type="queryResult.is_active ? 'success' : 'warning'" size="small">
                  {{ queryResult.is_active ? t("donate.query.activated") : t("donate.query.pendingActivation") }}
                </n-tag>
              </div>
              <div v-if="queryResult.donated_at" class="result-item">
                <span class="label">{{ t("donate.query.donatedAt") }}:</span>
                <span class="value">{{ new Date(queryResult.donated_at).toLocaleString() }}</span>
              </div>
            </div>
            <div v-else class="result-not-found">
              {{ t("donate.query.notFound") }}
            </div>
          </div>
        </div>
      </n-tab-pane>
    </n-tabs>

    <n-modal v-model:show="showSuccessModal" :mask-closable="false" :closable="false">
      <div class="result-modal">
        <div class="result-modal-header">
          <span class="success-icon">&#10004;</span>
          <h3>{{ t("donate.result.title") }}</h3>
        </div>

        <div class="result-summary">{{ resultSummary }}</div>

        <!-- Access Key Section -->
        <div v-if="generatedAccessKey" class="access-key-section">
          <div class="access-key-label">{{ t("donate.result.accessKeyLabel") }}</div>
          <div class="access-key-box">
            <code class="access-key-value">{{ generatedAccessKey }}</code>
            <n-button size="small" quaternary @click="copyAccessKey">
              {{ t("home.copy") }}
            </n-button>
          </div>
          <div class="access-key-warning">{{ t("donate.result.accessKeyWarning") }}</div>
        </div>

        <div class="result-table">
          <div class="result-table-head">
            <span>{{ t("keys.table.key") }}</span>
            <span>{{ t("donate.query.alias") }}</span>
            <span>{{ t("keys.table.status") }}</span>
          </div>
          <div
            v-for="(item, idx) in submissionItems"
            :key="idx"
            class="result-table-row"
          >
            <span class="mono">{{ item.key_masked }}</span>
            <span>{{ item.alias }}</span>
            <n-tag
              :type="statusTagType(item.status)"
              size="small"
            >
              {{ statusLabel(item.status) }}
            </n-tag>
          </div>
        </div>

        <div class="remember-hint">{{ t("donate.result.rememberHint") }}</div>

        <div class="result-modal-footer">
          <n-button type="primary" :disabled="countdown > 0" @click="closeModal">
            {{ countdown > 0 ? t("donate.result.closeIn", { s: countdown }) : t("donate.result.close") }}
          </n-button>
        </div>
      </div>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onBeforeUnmount } from "vue";
import {
  NTabs, NTabPane, NInput, NButton, NRadioGroup, NRadioButton,
  NModal, NTag, useMessage
} from "naive-ui";
import axios from "axios";
import { writeClipboardText } from "../utils/clipboard";
import { t } from "../i18n";

const emit = defineEmits<{
  (e: "quota-updated"): void;
}>();

const message = useMessage();
const activeTab = ref("donate");
const mode = ref<"single" | "batch">("single");
const alias = ref("");
const singleKey = ref("");
const batchKeys = ref("");
const submitting = ref(false);

const queryKey = ref("");
const querying = ref(false);

interface QueryResultData {
  found: boolean;
  alias?: string;
  is_active?: boolean;
  donated_at?: string;
}
const queryResult = ref<QueryResultData | null>(null);

const showSuccessModal = ref(false);

interface DonationItem {
  key_masked: string;
  alias: string;
  status: string;
}
const submissionItems = ref<DonationItem[]>([]);
const resultStats = ref({ created: 0, duplicated: 0, invalid: 0, activated: 0 });
const generatedAccessKey = ref("");
const countdown = ref(0);
let timer: ReturnType<typeof setInterval> | null = null;

const isValidKey = (key: string) => key.trim().startsWith("tvly-");

const validBatchKeys = computed(() =>
  batchKeys.value
    .split("\n")
    .map((k) => k.trim())
    .filter((k) => isValidKey(k))
);

const hasInvalidInBatch = computed(() =>
  batchKeys.value.split("\n").some((k) => k.trim() !== "" && !isValidKey(k))
);

const canSubmitSingle = computed(
  () => alias.value.trim() !== "" && singleKey.value.trim() !== "" && isValidKey(singleKey.value)
);
const canSubmitBatch = computed(
  () => alias.value.trim() !== "" && validBatchKeys.value.length > 0
);

const resultSummary = computed(() => {
  const s = resultStats.value;
  const parts: string[] = [];
  if (s.activated > 0) parts.push(t("donate.result.summaryActivated", { count: s.activated }));
  if (s.duplicated > 0) parts.push(t("donate.result.summaryDuplicated", { count: s.duplicated }));
  if (s.invalid > 0) parts.push(t("donate.result.summaryInvalid", { count: s.invalid }));
  if (parts.length === 0) return t("donate.result.summaryAllNew", { created: s.created });
  return parts.join(t("donate.result.summarySeparator"));
});

function statusTagType(status: string): "success" | "info" | "error" | "warning" {
  switch (status) {
    case "created": return "success";
    case "duplicated": return "info";
    case "invalid_key": return "error";
    case "invalid": return "warning";
    default: return "info";
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case "created": return t("donate.result.statusCreated");
    case "duplicated": return t("donate.result.statusDuplicated");
    case "invalid_key": return t("donate.result.statusInvalidKey");
    case "invalid": return t("donate.result.statusInvalid");
    default: return status;
  }
}

async function copyAccessKey() {
  try {
    await writeClipboardText(generatedAccessKey.value);
    message.success(t("common.copiedToClipboard"));
  } catch {
    message.error(t("common.copyFailed"));
  }
}

async function handleDonate() {
  const keys =
    mode.value === "single"
      ? [singleKey.value.trim()]
      : validBatchKeys.value;
  if (keys.length === 0) return;
  if (alias.value.trim() === "") {
    message.warning(t("donate.aliasRequired"));
    return;
  }

  submitting.value = true;
  try {
    const res = await axios.post("/api/public/donate", {
      keys,
      alias: alias.value.trim(),
    });
    submissionItems.value = res.data.items;
    resultStats.value = {
      created: res.data.created ?? 0,
      duplicated: res.data.duplicated ?? 0,
      invalid: res.data.invalid ?? 0,
      activated: res.data.activated ?? 0,
    };
    generatedAccessKey.value = res.data.access_key ?? "";
    emit("quota-updated");
    showSuccessModal.value = true;
    startCountdown();
  } catch (err: any) {
    if (err.response?.status === 429) {
      message.error(t("donate.rateLimited"));
    } else {
      message.error(t("common.createFailed"));
    }
  } finally {
    submitting.value = false;
  }
}

async function handleQuery() {
  if (!queryKey.value.trim()) return;
  querying.value = true;
  queryResult.value = null;
  try {
    const res = await axios.post("/api/public/donate/query", {
      key: queryKey.value.trim(),
    });
    queryResult.value = res.data;
  } catch (err: any) {
    if (err.response?.status === 429) {
      message.error(t("donate.rateLimited"));
    } else {
      message.error(t("common.createFailed"));
    }
  } finally {
    querying.value = false;
  }
}

function startCountdown() {
  countdown.value = 10;
  if (timer) clearInterval(timer);
  timer = setInterval(() => {
    countdown.value--;
    if (countdown.value <= 0 && timer) {
      clearInterval(timer);
      timer = null;
    }
  }, 1000);
}

function closeModal() {
  showSuccessModal.value = false;
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
  alias.value = "";
  singleKey.value = "";
  batchKeys.value = "";
  generatedAccessKey.value = "";
}

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});
</script>

<style scoped>
.key-donation {
  margin-top: 40px;
  padding: 32px;
  border: 1px solid rgba(128, 128, 128, 0.2);
  border-radius: 16px;
}

.donation-header {
  display: flex;
  gap: 16px;
  margin-bottom: 24px;
  align-items: flex-start;
}

.header-icon {
  font-size: 28px;
  line-height: 1;
  flex-shrink: 0;
}

.header-text .title {
  font-size: 22px;
  font-weight: 700;
  margin: 0 0 4px;
  color: var(--n-text-color);
}

.header-text .desc {
  font-size: 14px;
  color: var(--n-text-color-3);
  margin: 0;
  line-height: 1.5;
}

.tab-content {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding-top: 20px;
}

.form-item {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.item-label {
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 8px;
  color: var(--n-text-color-2);
}

.required {
  color: var(--n-error-color);
}

.single-row {
  display: flex;
  gap: 12px;
}

.batch-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 12px;
}

.count-info {
  font-size: 13px;
  color: var(--n-text-color-3);
}

.count-info.has-invalid {
  color: var(--n-warning-color);
}

.donate-hint {
  padding: 12px 16px;
  border-left: 3px solid var(--n-primary-color);
  background-color: var(--n-action-color);
  border-radius: 0 8px 8px 0;
}

.hint-content {
  font-size: 13px;
  color: var(--n-text-color-3);
  line-height: 1.6;
}

.query-result-wrapper {
  padding: 16px;
  background-color: var(--n-action-color);
  border-radius: 8px;
}

.result-item {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
  font-size: 14px;
}

.result-item:last-child {
  margin-bottom: 0;
}

.result-item .label {
  color: var(--n-text-color-3);
}

.result-item .value {
  font-weight: 500;
}

.result-not-found {
  font-size: 14px;
  color: var(--n-text-color-3);
}

/* Modal */
.result-modal {
  background: var(--n-color);
  border-radius: 12px;
  padding: 32px;
  width: 600px;
  max-width: 90vw;
}

.result-modal-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 20px;
}

.result-modal-header .success-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background-color: var(--n-success-color);
  color: #fff;
  font-size: 16px;
  flex-shrink: 0;
}

.result-modal-header h3 {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: var(--n-text-color);
}

.result-summary {
  margin-bottom: 16px;
  font-size: 14px;
  font-weight: 500;
  color: var(--n-text-color-2);
}

/* Access Key Section */
.access-key-section {
  margin-bottom: 20px;
  padding: 16px;
  background-color: var(--n-success-color-suppl);
  border: 1px solid var(--n-success-color);
  border-radius: 8px;
}

.access-key-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--n-text-color);
  margin-bottom: 8px;
}

.access-key-box {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  background-color: var(--n-code-color);
  border-radius: 6px;
  border: 1px solid var(--n-border-color);
}

.access-key-value {
  flex: 1;
  font-family: var(--n-font-family-mono);
  font-size: 14px;
  word-break: break-all;
  color: var(--n-text-color);
}

.access-key-warning {
  font-size: 12px;
  color: var(--n-warning-color);
  margin-top: 8px;
  font-weight: 500;
}

.result-table {
  border: 1px solid var(--n-border-color);
  border-radius: 8px;
  overflow: hidden;
  margin-bottom: 16px;
}

.result-table-head {
  display: grid;
  grid-template-columns: 1fr 1fr 100px;
  gap: 12px;
  padding: 10px 16px;
  background-color: var(--n-action-color);
  font-size: 13px;
  font-weight: 600;
  color: var(--n-text-color-3);
}

.result-table-row {
  display: grid;
  grid-template-columns: 1fr 1fr 100px;
  gap: 12px;
  padding: 10px 16px;
  font-size: 13px;
  border-top: 1px solid var(--n-border-color);
  align-items: center;
}

.mono {
  font-family: var(--n-font-family-mono);
}

.remember-hint {
  font-size: 13px;
  color: var(--n-text-color-3);
  font-style: italic;
  line-height: 1.5;
  margin-bottom: 20px;
}

.result-modal-footer {
  display: flex;
  justify-content: flex-end;
}

@media (max-width: 600px) {
  .key-donation {
    margin-top: 24px;
    padding: 16px;
    border-radius: 12px;
  }
  .donation-header {
    gap: 12px;
    margin-bottom: 16px;
  }
  .header-icon {
    font-size: 22px;
  }
  .header-text .title {
    font-size: 18px;
  }
  .header-text .desc {
    font-size: 13px;
  }
  .tab-content {
    gap: 14px;
    padding-top: 14px;
  }
  .single-row {
    flex-direction: column;
  }
  .donate-hint {
    padding: 10px 12px;
  }
  .hint-content {
    font-size: 12px;
  }
  .result-modal {
    padding: 20px;
    width: 95vw;
    border-radius: 10px;
  }
  .result-modal-header h3 {
    font-size: 17px;
  }
  .access-key-section {
    padding: 12px;
  }
  .access-key-value {
    font-size: 12px;
  }
  .result-table-head,
  .result-table-row {
    grid-template-columns: 1fr 80px 72px;
    gap: 6px;
    padding: 8px 10px;
    font-size: 12px;
  }
  .result-table-head span:nth-child(2),
  .result-table-row span:nth-child(2) {
    display: none;
  }
  .result-table-head,
  .result-table-row {
    grid-template-columns: 1fr 72px;
  }
  .remember-hint {
    font-size: 12px;
  }
}
</style>
