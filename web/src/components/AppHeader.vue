<template>
  <n-layout-header bordered class="app-header">
    <div class="logo-container" @click="emit('back-home')">
      <n-icon size="24" :component="GitNetworkOutline" class="logo-icon" />
      <span class="logo-text">Tavily Proxy</span>
    </div>
    <div style="flex: 1"></div>
    <n-space align="center">
      <n-button
        v-if="screen === 'admin' && isMobile"
        quaternary
        circle
        :aria-label="t('app.openMenu')"
        @click="emit('open-mobile-menu')"
      >
        <template #icon>
          <n-icon :component="MenuOutline" />
        </template>
      </n-button>
      <n-button
        quaternary
        circle
        :aria-label="t('app.toggleTheme')"
        @click="emit('toggle-theme')"
      >
        <template #icon>
          <n-icon :component="isDark ? SunnyOutline : MoonOutline" />
        </template>
      </n-button>
      <n-dropdown :options="languageOptions" @select="onSelectLanguage">
        <n-button quaternary size="small">
          <template #icon>
            <n-icon :component="LanguageOutline" />
          </template>
          {{ locale === "zh-CN" ? "中文" : "EN" }}
        </n-button>
      </n-dropdown>
      <template v-if="screen === 'home'">
        <n-button
          v-if="hasSession"
          size="small"
          type="primary"
          @click="emit('enter-app')"
        >
          <template #icon>
            <n-icon :component="LogInOutline" />
          </template>
          {{ t("home.enter") }}
        </n-button>
        <n-button
          v-else
          size="small"
          type="primary"
          @click="emit('request-login')"
        >
          <template #icon>
            <n-icon :component="LogInOutline" />
          </template>
          {{ t("home.login") }}
        </n-button>
      </template>
      <template v-else>
        <n-button size="small" secondary @click="emit('back-home')">
          <template #icon>
            <n-icon :component="HomeOutline" />
          </template>
          {{ t("home.backToHome") }}
        </n-button>
        <n-button size="small" secondary type="primary" @click="emit('logout')">
          <template #icon>
            <n-icon :component="LogOutOutline" />
          </template>
          {{ t("app.menu.logout") }}
        </n-button>
      </template>
    </n-space>
  </n-layout-header>
</template>

<script setup lang="ts">
import { NButton, NDropdown, NIcon, NLayoutHeader, NSpace } from "naive-ui";
import {
  GitNetworkOutline,
  HomeOutline,
  LanguageOutline,
  LogInOutline,
  LogOutOutline,
  MenuOutline,
  MoonOutline,
  SunnyOutline,
} from "@vicons/ionicons5";
import { locale, setLocale, t } from "../i18n";

defineProps<{
  isDark: boolean;
  screen: "home" | "admin" | "donor";
  isMobile: boolean;
  hasSession: boolean;
}>();

const emit = defineEmits<{
  (e: "toggle-theme"): void;
  (e: "request-login"): void;
  (e: "enter-app"): void;
  (e: "back-home"): void;
  (e: "logout"): void;
  (e: "open-mobile-menu"): void;
}>();

const languageOptions = [
  { label: "English", key: "en" },
  { label: "中文", key: "zh-CN" },
];

function onSelectLanguage(key: string | number) {
  if (key === "en" || key === "zh-CN") {
    setLocale(key);
  }
}
</script>

<style scoped>
.app-header {
  height: 56px;
  display: flex;
  align-items: center;
  padding: 0 24px;
  border-bottom: 2px solid var(--comic-ink);
  z-index: 10;
}

.logo-container {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
}

.logo-icon {
  color: #0ea5e9;
}

.logo-text {
  font-size: 20px;
  font-weight: 800;
  font-family: var(--comic-font-display);
  letter-spacing: -0.5px;
  background: linear-gradient(135deg, #0ea5e9, #2563eb);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

@media (max-width: 900px) {
  .app-header {
    padding: 0 12px;
  }

  .logo-text {
    font-size: 16px;
  }
}

@media (max-width: 640px) {
  .app-header {
    padding: 0 10px;
  }
}
</style>
