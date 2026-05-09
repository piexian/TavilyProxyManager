<template>
  <n-config-provider
    :theme="theme"
    :theme-overrides="currentOverrides"
    :locale="naiveLocale"
    :date-locale="naiveDateLocale"
  >
    <n-global-style />
    <n-message-provider>
      <n-layout style="min-height: 100vh" class="root-layout">
        <n-layout-header bordered class="app-header">
          <div class="logo-container" style="cursor: pointer" @click="showAdmin = false">
            <n-icon size="24" :component="GlobeOutline" class="logo-icon" />
            <span class="logo-text">Tavily Proxy</span>
          </div>
          <div style="flex: 1"></div>
          <n-space align="center">
            <n-button
              v-if="showAdmin && isMobile"
              quaternary
              circle
              @click="showMobileMenu = true"
            >
              <template #icon>
                <n-icon :component="MenuOutline" />
              </template>
            </n-button>
            <n-button quaternary circle @click="toggleTheme">
              <template #icon>
                <n-icon
                  :component="theme === null ? MoonOutline : SunnyOutline"
                />
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
            <n-button
              v-if="!showAdmin"
              size="small"
              type="primary"
              @click="enterAdmin"
            >
              <template #icon>
                <n-icon :component="SettingsOutline" />
              </template>
              {{ t("home.adminPanel") }}
            </n-button>
            <template v-else>
              <n-button size="small" secondary @click="showAdmin = false">
                <template #icon>
                  <n-icon :component="HomeOutline" />
                </template>
                {{ t("home.backToHome") }}
              </n-button>
              <n-button size="small" @click="logout" secondary type="primary">
                <template #icon>
                  <n-icon :component="LogOutOutline" />
                </template>
                {{ t("app.menu.logout") }}
              </n-button>
            </template>
          </n-space>
        </n-layout-header>

        <!-- Home view (no sidebar) -->
        <n-layout
          v-if="!showAdmin"
          position="absolute"
          style="top: 56px; bottom: 0"
        >
          <n-layout-content
            content-style="padding: 16px;"
            :native-scrollbar="false"
            class="app-content"
          >
            <HomeView />
          </n-layout-content>
        </n-layout>

        <!-- Admin view (with sidebar) -->
        <n-layout
          v-else
          :has-sider="!isMobile"
          position="absolute"
          style="top: 56px; bottom: 0"
        >
          <n-layout-sider
            v-if="!isMobile"
            bordered
            collapse-mode="width"
            :collapsed-width="64"
            :width="220"
            show-trigger
            :collapsed="collapsed"
            @collapse="collapsed = true"
            @expand="collapsed = false"
            class="app-sider"
          >
            <div class="sider-content">
              <n-menu
                v-model:value="active"
                :collapsed="collapsed"
                :collapsed-width="64"
                :collapsed-icon-size="22"
                :options="menuOptions"
                @update:value="handleMenuUpdate"
                class="sider-menu"
              />
            </div>
          </n-layout-sider>
          <n-layout-content
            content-style="padding: 16px;"
            :native-scrollbar="false"
            class="app-content"
          >
            <div class="main-container">
              <DashboardView
                v-if="active === 'dashboard'"
                :refresh-nonce="dashboardRefreshNonce"
              />
              <KeyManagementView v-else-if="active === 'keys'" />
              <AccessKeysView v-else-if="active === 'access-keys'" />
              <LogsView v-else-if="active === 'logs'" />
              <SettingsView v-else />
            </div>
          </n-layout-content>
        </n-layout>

        <n-drawer
          v-model:show="showMobileMenu"
          placement="left"
          :width="280"
          :auto-focus="false"
        >
          <n-drawer-content :title="t('app.title')" closable body-content-style="padding: 12px;">
            <n-menu
              v-model:value="active"
              :options="menuOptions"
              @update:value="handleMenuUpdate"
            />
          </n-drawer-content>
        </n-drawer>
      </n-layout>

      <MasterKeyModal
        :show="showAuthModal"
        :initial-value="draftKey"
        :error="authError"
        :cancelable="true"
        @submit="saveKey"
        @cancel="showAdmin = false"
      />
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref } from "vue";
import {
  NButton,
  NConfigProvider,
  NDrawer,
  NDrawerContent,
  NDropdown,
  NGlobalStyle,
  NIcon,
  NLayout,
  NLayoutContent,
  NLayoutHeader,
  NLayoutSider,
  NMenu,
  NMessageProvider,
  NSpace,
  darkTheme,
  dateEnUS,
  dateZhCN,
  enUS,
  type GlobalThemeOverrides,
  zhCN,
} from "naive-ui";
import {
  BarChartOutline,
  GlobeOutline,
  HomeOutline,
  KeyOutline,
  LanguageOutline,
  LockClosedOutline,
  ListOutline,
  LogOutOutline,
  MenuOutline,
  MoonOutline,
  SettingsOutline,
  SunnyOutline,
} from "@vicons/ionicons5";
import MasterKeyModal from "./components/MasterKeyModal.vue";
import HomeView from "./views/HomeView.vue";
import DashboardView from "./views/DashboardView.vue";
import KeyManagementView from "./views/KeyManagementView.vue";
import AccessKeysView from "./views/AccessKeysView.vue";
import LogsView from "./views/LogsView.vue";
import SettingsView from "./views/SettingsView.vue";
import { api, clearMasterKey, getMasterKey, setMasterKey } from "./api/client";
import { locale, setLocale, t } from "./i18n";

const showAdmin = ref(false);
const active = ref<"dashboard" | "keys" | "access-keys" | "logs" | "settings">("dashboard");
const collapsed = ref(false);
const theme = ref<any>(null);
const isMobile = ref(false);
const showMobileMenu = ref(false);
let mobileQuery: MediaQueryList | null = null;

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: "#6366f1",
    primaryColorHover: "#818cf8",
    primaryColorPressed: "#4f46e5",
    primaryColorSuppl: "#818cf8",
    infoColor: "#0ea5e9",
    infoColorHover: "#38bdf8",
    successColor: "#10b981",
    warningColor: "#f59e0b",
    errorColor: "#ef4444",
    borderRadius: "8px",
    fontFamily: "Inter, system-ui, -apple-system, sans-serif",
  },
  Card: {
    borderRadius: "12px",
    boxShadow: "0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1)",
  },
  Layout: {
    headerColor: "rgba(255, 255, 255, 0.8)",
    siderColor: "rgba(255, 255, 255, 0.6)",
  },
  Menu: {
    itemBorderRadius: "8px",
    itemColorActive: "rgba(99, 102, 241, 0.1)",
    itemColorActiveHover: "rgba(99, 102, 241, 0.15)",
    itemTextColorActive: "#6366f1",
    itemIconColorActive: "#6366f1",
  },
};

const darkThemeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: "#818cf8",
    primaryColorHover: "#a5b4fc",
    primaryColorPressed: "#6366f1",
    primaryColorSuppl: "#a5b4fc",
    borderRadius: "8px",
  },
  Layout: {
    headerColor: "#18181c",
    siderColor: "#18181c",
  },
  Menu: {
    itemColorActive: "rgba(129, 140, 248, 0.15)",
    itemTextColorActive: "#818cf8",
    itemIconColorActive: "#818cf8",
  },
};

const currentOverrides = computed(() => {
  return theme.value === null
    ? themeOverrides
    : { ...themeOverrides, ...darkThemeOverrides };
});

const naiveLocale = computed(() => {
  return locale.value === "zh-CN" ? zhCN : enUS;
});

const naiveDateLocale = computed(() => {
  return locale.value === "zh-CN" ? dateZhCN : dateEnUS;
});

function renderIcon(icon: any) {
  return () => h(NIcon, null, { default: () => h(icon) });
}

const menuOptions = computed(() => [
  {
    label: t("app.menu.dashboard"),
    key: "dashboard",
    icon: renderIcon(BarChartOutline),
  },
  { label: t("app.menu.keys"), key: "keys", icon: renderIcon(KeyOutline) },
  { label: t("app.menu.accessKeys"), key: "access-keys", icon: renderIcon(LockClosedOutline) },
  { label: t("app.menu.logs"), key: "logs", icon: renderIcon(ListOutline) },
  {
    label: t("app.menu.settings"),
    key: "settings",
    icon: renderIcon(SettingsOutline),
  },
]);

const languageOptions = [
  { label: "English", key: "en" },
  { label: "中文", key: "zh-CN" },
];

function onSelectLanguage(key: string | number) {
  if (key === "en" || key === "zh-CN") {
    setLocale(key);
  }
}

const draftKey = ref("");
const showAuthModal = computed(() => showAdmin.value && !getMasterKey());
const authError = ref("");
const dashboardRefreshNonce = ref(0);

function enterAdmin() {
  if (getMasterKey()) {
    showAdmin.value = true;
  } else {
    showAdmin.value = true; // triggers showAuthModal
  }
}

async function verifyKey() {
  try {
    await api.get("/api/stats");
    return true;
  } catch {
    return false;
  }
}

async function saveKey(value: string) {
  authError.value = "";
  setMasterKey(value);
  const ok = await verifyKey();
  if (!ok) {
    clearMasterKey();
    draftKey.value = value;
    authError.value = t("app.invalidMasterKey");
  } else {
    draftKey.value = "";
    dashboardRefreshNonce.value += 1;
  }
}

function logout() {
  clearMasterKey();
  draftKey.value = "";
  authError.value = "";
  showAdmin.value = false;
  showMobileMenu.value = false;
}

function toggleTheme() {
  theme.value = theme.value === null ? darkTheme : null;
  localStorage.setItem("theme", theme.value === null ? "light" : "dark");
}

function syncMobileState() {
  if (!mobileQuery) return;
  isMobile.value = mobileQuery.matches;
  if (isMobile.value) {
    collapsed.value = true;
  } else {
    showMobileMenu.value = false;
  }
}

function handleMenuUpdate() {
  if (isMobile.value) {
    showMobileMenu.value = false;
  }
}

onMounted(() => {
  const savedTheme = localStorage.getItem("theme");
  if (savedTheme === "dark") {
    theme.value = darkTheme;
  }

  mobileQuery = window.matchMedia("(max-width: 900px)");
  syncMobileState();
  mobileQuery.addEventListener("change", syncMobileState);

  window.addEventListener("auth-required", () => {
    const current = getMasterKey();
    clearMasterKey();
    draftKey.value = current;
    authError.value = "";
    // stay on admin to show auth modal
  });
});

onBeforeUnmount(() => {
  mobileQuery?.removeEventListener("change", syncMobileState);
});
</script>

<style scoped>
.root-layout {
  background-color: v-bind("theme === null ? '#f8fafc' : '#101014'");
}

.app-header {
  height: 56px;
  display: flex;
  align-items: center;
  padding: 0 24px;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  z-index: 10;
  backdrop-filter: blur(8px);
}

.logo-container {
  display: flex;
  align-items: center;
  gap: 10px;
}

.logo-icon {
  color: #6366f1;
}

.logo-text {
  font-size: 18px;
  font-weight: 700;
  letter-spacing: -0.5px;
  background: linear-gradient(135deg, #6366f1, #818cf8);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.app-sider {
  background-color: v-bind(
    "theme === null ? 'rgba(255, 255, 255, 0.6)' : '#18181c'"
  );
}

.sider-content {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.sider-menu {
  flex: 1;
}

.sider-footer {
  padding: 8px;
}

.app-content {
  background-color: transparent;
}

.main-container {
  max-width: 1200px;
  margin: 0 auto;
}

:deep(.n-layout-sider) {
  background-color: transparent;
}

@media (max-width: 900px) {
  .app-header {
    padding: 0 12px;
  }

  .logo-text {
    font-size: 16px;
  }

  .main-container {
    max-width: 100%;
  }

  :deep(.page-header) {
    flex-direction: column;
    align-items: flex-start !important;
    gap: 12px;
  }

  :deep(.page-header > .n-space) {
    width: 100%;
    flex-wrap: wrap;
    justify-content: flex-start !important;
  }

  :deep(.page-header > .n-space .n-base-selection),
  :deep(.page-header > .n-space .n-input-number) {
    width: 100% !important;
  }

  :deep(.pagination-container) {
    justify-content: flex-start !important;
    overflow-x: auto;
  }
}

@media (max-width: 640px) {
  .app-header {
    padding: 0 10px;
  }

  :deep(.page-title) {
    font-size: 20px !important;
  }

  :deep(.page-subtitle) {
    font-size: 13px !important;
    line-height: 1.5;
  }

  :deep(.table-card) {
    overflow: hidden;
  }
}
</style>
