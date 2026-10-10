<script setup lang="ts">
import { ref } from "vue";
import {
  NAlert,
  NButton,
  NIcon,
  NInput,
  NInputGroup,
  NModal,
  NPopconfirm,
  NSpace,
  useMessage,
} from "naive-ui";
import { CopyOutline, KeyOutline, RefreshOutline } from "@vicons/ionicons5";
import { api, setMasterKey } from "../../api/client";
import { writeClipboardText } from "../../utils/clipboard";
import { t } from "../../i18n";
import type { PortalMe } from "../../types";

const props = defineProps<{ me: PortalMe | null }>();
const emit = defineEmits<{ (e: "reset"): void }>();

const message = useMessage();
const resetting = ref(false);
const showReveal = ref(false);
const newKey = ref("");

async function resetKey() {
  resetting.value = true;
  try {
    const { data } = await api.post<{ access_key: string }>("/api/portal/reset-key");
    // 立即把新 token 写入本地，旧 token 已失效但会话因此不掉线。
    setMasterKey(data.access_key);
    newKey.value = data.access_key;
    showReveal.value = true;
    message.success(t("portal.security.resetDone"));
    emit("reset");
  } catch (err: any) {
    message.error(err?.response?.data?.error ?? t("portal.security.resetFailed"));
  } finally {
    resetting.value = false;
  }
}

async function copyNewKey() {
  try {
    await writeClipboardText(newKey.value);
    message.success(t("common.copiedToClipboard"));
  } catch {
    message.error(t("common.copyFailed"));
  }
}
</script>

<template>
  <div class="portal-security">
    <div class="field">
      <div class="field-label">{{ t("portal.security.aliasLabel") }}</div>
      <div class="field-value">{{ props.me?.alias ?? "-" }}</div>
    </div>

    <div class="field">
      <div class="field-label">{{ t("portal.security.keyLabel") }}</div>
      <code class="masked-key">{{ props.me?.key ?? "-" }}</code>
    </div>

    <n-alert type="warning" :show-icon="true" class="reset-warning">
      {{ t("portal.security.resetWarning") }}
    </n-alert>

    <n-popconfirm @positive-click="resetKey">
      <template #trigger>
        <n-button type="error" :loading="resetting" secondary>
          <template #icon><n-icon :component="RefreshOutline" /></template>
          {{ t("portal.security.resetButton") }}
        </n-button>
      </template>
      {{ t("portal.security.resetConfirm") }}
    </n-popconfirm>

    <n-modal
      v-model:show="showReveal"
      preset="card"
      :title="t('portal.security.revealTitle')"
      style="max-width: 540px"
      :mask-closable="false"
    >
      <n-space vertical size="large">
        <n-alert type="warning" :show-icon="true">
          {{ t("portal.security.revealWarning") }}
        </n-alert>
        <div>
          <div class="field-label">{{ t("portal.security.newKeyLabel") }}</div>
          <n-input-group>
            <n-input :value="newKey" readonly>
              <template #prefix><n-icon :component="KeyOutline" /></template>
            </n-input>
            <n-button type="primary" ghost @click="copyNewKey">
              <template #icon><n-icon :component="CopyOutline" /></template>
            </n-button>
          </n-input-group>
        </div>
      </n-space>
      <template #footer>
        <n-space justify="end">
          <n-button type="primary" @click="showReveal = false">
            {{ t("common.dismiss") }}
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<style scoped>
.portal-security {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 560px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.field-label {
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--n-text-color-3);
}

.field-value {
  font-size: 16px;
  font-weight: 600;
}

.masked-key {
  font-family: var(--n-font-family-mono);
  font-size: 14px;
  padding: 8px 12px;
  border-radius: 8px;
  border: 2px solid var(--comic-ink);
  background: rgba(14, 165, 233, 0.06);
  word-break: break-all;
}

.reset-warning {
  border-radius: 10px;
}
</style>
