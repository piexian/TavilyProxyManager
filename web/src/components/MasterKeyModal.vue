<template>
  <n-modal
    :show="show"
    preset="card"
    :title="t('auth.title')"
    :mask-closable="false"
    :closable="false"
    style="max-width: 480px"
    class="auth-modal"
  >
    <n-space vertical size="large">
      <div class="lang-switch">
        <n-dropdown :options="languageOptions" @select="onSelectLanguage">
          <n-button quaternary size="small">
            <template #icon>
              <n-icon :component="LanguageOutline" />
            </template>
            {{ locale === "zh-CN" ? "中文" : "EN" }}
          </n-button>
        </n-dropdown>
      </div>
      <div class="auth-header">
        <n-icon size="48" :component="LockClosedOutline" class="auth-icon" />
        <div class="auth-title">{{ t("auth.welcome") }}</div>
        <div class="auth-subtitle">
          {{ t("auth.subtitle") }}
        </div>
      </div>

      <n-alert v-if="error" type="error" closable class="error-alert">
        {{ error }}
      </n-alert>

      <n-form-item :label="t('auth.masterKeyLabel')" label-placement="top">
        <n-input
          v-model:value="value"
          type="password"
          :placeholder="t('auth.masterKeyPlaceholder')"
          show-password-on="mousedown"
          size="large"
          @keyup.enter="onSubmit"
          autofocus
        >
          <template #prefix>
            <n-icon :component="KeyOutline" />
          </template>
        </n-input>
      </n-form-item>

      <n-button
        type="primary"
        size="large"
        block
        :disabled="!value.trim()"
        @click="onSubmit"
      >
        {{ t("auth.accessDashboard") }}
      </n-button>

      <n-button
        v-if="cancelable"
        size="large"
        block
        quaternary
        @click="emit('cancel')"
      >
        {{ t("common.cancel") }}
      </n-button>

      <div class="auth-footer">
        {{ t("auth.footer") }}
      </div>
    </n-space>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import {
  NAlert,
  NButton,
  NDropdown,
  NFormItem,
  NIcon,
  NInput,
  NModal,
  NSpace,
} from "naive-ui";
import { KeyOutline, LanguageOutline, LockClosedOutline } from "@vicons/ionicons5";
import { locale, setLocale, t } from "../i18n";

const props = defineProps<{
  show: boolean;
  initialValue?: string;
  error?: string;
  cancelable?: boolean;
}>();

const emit = defineEmits<{
  (e: "submit", value: string): void;
  (e: "cancel"): void;
}>();

const value = ref(props.initialValue ?? "");

const languageOptions = [
  { label: "English", key: "en" },
  { label: "中文", key: "zh-CN" },
];

function onSelectLanguage(key: string | number) {
  if (key === "en" || key === "zh-CN") {
    setLocale(key);
  }
}

watch(
  () => props.initialValue,
  (v) => {
    if (typeof v === "string") value.value = v;
  }
);

function onSubmit() {
  if (!value.value.trim()) return;
  emit("submit", value.value.trim());
}
</script>

<style scoped>
.auth-modal {
  border-radius: 20px;
}

.lang-switch {
  display: flex;
  justify-content: flex-end;
}

.auth-header {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.auth-icon {
  color: #18a058;
  margin-bottom: 8px;
}

.auth-title {
  font-size: 22px;
  font-weight: 700;
}

.auth-subtitle {
  color: #888;
  text-align: center;
  font-size: 14px;
}

.error-alert {
  border-radius: 8px;
}

.auth-footer {
  text-align: center;
  color: #bbb;
  font-size: 12px;
  margin-top: 8px;
}
</style>
