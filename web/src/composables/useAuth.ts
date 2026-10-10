import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import axios from "axios";
import {
  clearMasterKey,
  getMasterKey,
  getRole,
  setMasterKey,
  setRole,
  type AuthRole,
} from "../api/client";
import { t } from "../i18n";

export type Screen = "home" | "admin" | "donor";

/**
 * 统一登录编排：一个登录弹窗同时受理 Master Key 与捐赠者访问密钥。
 * 先探测 Master Key（/api/stats）→ 管理员；否则探测访问密钥（/api/portal/me）→ 捐赠者门户；
 * 都失败则提示密钥无效。探测使用裸 axios（绕过全局 401 拦截器），避免探测失败清空会话状态。
 */
export function useAuth() {
  const screen = ref<Screen>("home");
  const loginRequested = ref(false);
  const draftKey = ref("");
  const authError = ref("");
  const dashboardRefreshNonce = ref(0);
  const portalRefreshNonce = ref(0);

  // 弹窗仅由"是否请求登录"决定，不受本地残留 token 影响（否则旧版遗留的 token 会压制弹窗）。
  const showAuthModal = computed(() => loginRequested.value);

  // 响应式会话标记：随 setMasterKey/setRole/clearMasterKey 变化而更新（client.ts 内部用 ref 持有）。
  // 决定右上角按钮显示「登录」还是「进入」。
  const role = ref<AuthRole>(getRole());
  const masterKey = ref<string>(getMasterKey());
  const hasSession = computed(
    () =>
      !!masterKey.value && (role.value === "admin" || role.value === "donor"),
  );
  // 集中写会话，保证 hasSession 同步刷新。
  function applySession(token: string, r: AuthRole) {
    setMasterKey(token);
    setRole(r);
    masterKey.value = token;
    role.value = r;
  }
  function dropSession() {
    clearMasterKey();
    masterKey.value = "";
    role.value = "";
  }

  // 裸请求探测：仅用 Bearer 头校验密钥类型，不写入全局 token，也不触发 401 拦截器。
  // 返回 code 以便区分「禁用登录」(login_disabled) 与普通无效，给出精准提示。
  async function probe(
    url: string,
    token: string,
  ): Promise<{ ok: boolean; code?: string }> {
    try {
      await axios.get(url, { headers: { Authorization: `Bearer ${token}` } });
      return { ok: true };
    } catch (err: any) {
      return { ok: false, code: err?.response?.data?.error };
    }
  }

  // 点击「登录」：弹出登录框（不论是否已有会话——「进入」走 enterApp）。
  function requestLogin() {
    loginRequested.value = true;
  }

  // 点击「进入」：已持有有效会话则直接进入对应界面，否则退化为弹出登录框。
  function enterApp() {
    if (masterKey.value && (role.value === "admin" || role.value === "donor")) {
      screen.value = role.value;
    } else {
      loginRequested.value = true;
    }
  }

  async function saveKey(value: string) {
    authError.value = "";
    const token = value.trim();
    if (!token) return;

    let detectedRole: AuthRole = "";
    if ((await probe("/api/stats", token)).ok) {
      detectedRole = "admin";
    } else {
      const portal = await probe("/api/portal/me", token);
      if (portal.ok) {
        detectedRole = "donor";
      } else if (portal.code === "login_disabled") {
        // 密钥本身有效（可调 API/MCP），但被管理员关闭了门户登录。
        dropSession();
        draftKey.value = value;
        authError.value = t("app.loginDisabled");
        return;
      }
    }

    if (detectedRole === "") {
      dropSession();
      draftKey.value = value;
      authError.value = t("app.invalidKey");
      return;
    }

    applySession(token, detectedRole);
    draftKey.value = "";
    loginRequested.value = false;
    screen.value = detectedRole;
    if (detectedRole === "admin") dashboardRefreshNonce.value += 1;
    else portalRefreshNonce.value += 1;
  }

  function cancelLogin() {
    loginRequested.value = false;
    authError.value = "";
  }

  function goHome() {
    screen.value = "home";
  }

  function logout() {
    dropSession();
    draftKey.value = "";
    authError.value = "";
    loginRequested.value = false;
    screen.value = "home";
  }

  // 后端 401 时 client.ts 会清除 token 并派发 auth-required，这里弹回登录框（保留当前界面作为背景）。
  function handleAuthRequired() {
    dropSession();
    draftKey.value = "";
    authError.value = "";
    loginRequested.value = true;
  }

  onMounted(async () => {
    window.addEventListener("auth-required", handleAuthRequired);
    const token = getMasterKey();
    if (!token) return;
    // 已知角色：仅恢复会话状态（让「进入」按钮可直接用），但停留在首页——
    // 不自动进入管理/门户界面（用户要求：有登录状态也不自动进入）。
    const existing = getRole();
    if (existing === "admin" || existing === "donor") {
      role.value = existing;
      masterKey.value = token;
      return;
    }
    // 残留 token 但无角色（多为旧版升级遗留）：探测恢复角色；失效则清除，避免卡住登录。
    if ((await probe("/api/stats", token)).ok) {
      applySession(token, "admin");
    } else if ((await probe("/api/portal/me", token)).ok) {
      applySession(token, "donor");
    } else {
      dropSession();
    }
  });
  onBeforeUnmount(() =>
    window.removeEventListener("auth-required", handleAuthRequired),
  );

  return {
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
  };
}
