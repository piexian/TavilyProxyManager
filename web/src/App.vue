<template>
  <n-config-provider
    :theme="theme"
    :theme-overrides="overrides"
    :locale="naiveLocale"
    :date-locale="naiveDateLocale"
  >
    <n-global-style />
    <n-message-provider>
      <n-layout style="min-height: 100vh" class="root-layout">
        <AppHeader
          :is-dark="isDark"
          :screen="screen"
          :is-mobile="isMobile"
          :has-session="hasSession"
          @toggle-theme="toggleTheme"
          @request-login="requestLogin"
          @enter-app="enterApp"
          @back-home="goHome"
          @logout="logout"
          @open-mobile-menu="showMobileMenu = true"
        />

        <!-- Home view (no sidebar) -->
        <n-layout
          v-if="screen === 'home'"
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
          v-else-if="screen === 'admin'"
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

        <!-- Donor portal (no sidebar) -->
        <n-layout
          v-else-if="screen === 'donor'"
          position="absolute"
          style="top: 56px; bottom: 0"
        >
          <n-layout-content
            content-style="padding: 16px;"
            :native-scrollbar="false"
            class="app-content"
          >
            <DonorPortalView :refresh-nonce="portalRefreshNonce" />
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
        @cancel="cancelLogin"
      />
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, h, ref, watch } from "vue";
import {
  NConfigProvider,
  NDrawer,
  NDrawerContent,
  NGlobalStyle,
  NIcon,
  NLayout,
  NLayoutContent,
  NLayoutSider,
  NMenu,
  NMessageProvider,
  dateEnUS,
  dateZhCN,
  enUS,
  zhCN,
} from "naive-ui";
import {
  BarChartOutline,
  KeyOutline,
  ListOutline,
  LockClosedOutline,
  SettingsOutline,
} from "@vicons/ionicons5";
import AppHeader from "./components/AppHeader.vue";
import MasterKeyModal from "./components/MasterKeyModal.vue";
import HomeView from "./views/HomeView.vue";
import DashboardView from "./views/DashboardView.vue";
import KeyManagementView from "./views/KeyManagementView.vue";
import AccessKeysView from "./views/AccessKeysView.vue";
import LogsView from "./views/LogsView.vue";
import SettingsView from "./views/SettingsView.vue";
import DonorPortalView from "./views/DonorPortalView.vue";
import { locale, t } from "./i18n";
import { useTheme } from "./composables/useTheme";
import { useMediaQuery } from "./composables/useResponsive";
import { useAuth } from "./composables/useAuth";

// 主题（状态/副作用集中在 composable）
const { theme, isDark, overrides, toggle: toggleTheme } = useTheme();

// 统一登录编排（home/admin/donor 三屏、登录弹窗、登入/登出、auth-required 事件）
const {
  screen,
  draftKey,
  authError,
  dashboardRefreshNonce,
  portalRefreshNonce,
  showAuthModal,
  hasSession,
  requestLogin,
  enterApp,
  saveKey,
  cancelLogin,
  goHome,
  logout,
} = useAuth();

// App shell 本地状态：当前视图、侧栏折叠、移动端抽屉
const active = ref<"dashboard" | "keys" | "access-keys" | "logs" | "settings">(
  "dashboard",
);
const collapsed = ref(false);
const isMobile = useMediaQuery("(max-width: 900px)");
const showMobileMenu = ref(false);

// 移动端默认折叠侧栏；切回桌面 / 离开 admin 时关闭抽屉
watch(
  isMobile,
  (mobile) => {
    if (mobile) collapsed.value = true;
    else showMobileMenu.value = false;
  },
  { immediate: true },
);
watch(screen, (s) => {
  if (s !== "admin") showMobileMenu.value = false;
});

const naiveLocale = computed(() => (locale.value === "zh-CN" ? zhCN : enUS));
const naiveDateLocale = computed(() =>
  locale.value === "zh-CN" ? dateZhCN : dateEnUS,
);

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
  {
    label: t("app.menu.accessKeys"),
    key: "access-keys",
    icon: renderIcon(LockClosedOutline),
  },
  { label: t("app.menu.logs"), key: "logs", icon: renderIcon(ListOutline) },
  {
    label: t("app.menu.settings"),
    key: "settings",
    icon: renderIcon(SettingsOutline),
  },
]);

function handleMenuUpdate() {
  if (isMobile.value) {
    showMobileMenu.value = false;
  }
}
</script>

<style scoped>
.root-layout {
  background-color: v-bind("theme === null ? '#f0f9ff' : '#0f172a'");
}

.app-sider {
  background-color: v-bind("theme === null ? '#ffffff' : '#1e293b'");
  border-right: 2px solid var(--comic-ink);
}

.sider-content {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.sider-menu {
  flex: 1;
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
