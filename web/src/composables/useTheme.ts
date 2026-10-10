import { computed, ref, watch, type Ref } from "vue";
import { darkTheme, type GlobalThemeOverrides } from "naive-ui";

// Clean Blue 清爽蓝白风：天蓝主色 + 圆润漫画字体 + 12px 圆角；
// 描边与硬阴影在 styles/comic.css 里全局处理。
const COMIC_FONT =
  '"Comic Neue", "Baloo 2", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", system-ui, -apple-system, sans-serif';

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: "#0ea5e9",
    primaryColorHover: "#38bdf8",
    primaryColorPressed: "#0284c7",
    primaryColorSuppl: "#38bdf8",
    infoColor: "#0ea5e9",
    infoColorHover: "#38bdf8",
    infoColorPressed: "#0284c7",
    successColor: "#22c55e",
    successColorHover: "#4ade80",
    successColorPressed: "#16a34a",
    warningColor: "#f59e0b",
    warningColorHover: "#fbbf24",
    warningColorPressed: "#d97706",
    errorColor: "#ef4444",
    errorColorHover: "#f87171",
    errorColorPressed: "#dc2626",
    borderRadius: "12px",
    borderRadiusSmall: "8px",
    fontFamily: COMIC_FONT,
    fontWeightStrong: "700",
    bodyColor: "#f0f9ff",
  },
  Card: {
    borderRadius: "12px",
    titleFontWeight: "700",
  },
  Layout: {
    color: "#f0f9ff",
    headerColor: "#ffffff",
    siderColor: "#ffffff",
  },
  Menu: {
    itemBorderRadius: "10px",
    itemColorActive: "rgba(14, 165, 233, 0.14)",
    itemColorActiveHover: "rgba(14, 165, 233, 0.2)",
    itemColorActiveCollapsed: "rgba(14, 165, 233, 0.14)",
    itemTextColorActive: "#0284c7",
    itemTextColorActiveHover: "#0284c7",
    itemIconColorActive: "#0ea5e9",
    itemIconColorActiveHover: "#0ea5e9",
  },
};

const darkThemeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: "#38bdf8",
    primaryColorHover: "#7dd3fc",
    primaryColorPressed: "#0ea5e9",
    primaryColorSuppl: "#7dd3fc",
    infoColor: "#38bdf8",
    successColor: "#4ade80",
    warningColor: "#fbbf24",
    errorColor: "#f87171",
    borderRadius: "12px",
    borderRadiusSmall: "8px",
    fontFamily: COMIC_FONT,
    fontWeightStrong: "700",
    bodyColor: "#0f172a",
  },
  Card: {
    color: "#1e293b",
    borderRadius: "12px",
    titleFontWeight: "700",
  },
  Layout: {
    color: "#0f172a",
    headerColor: "#1e293b",
    siderColor: "#1e293b",
  },
  Menu: {
    itemBorderRadius: "10px",
    itemColorActive: "rgba(56, 189, 248, 0.2)",
    itemColorActiveHover: "rgba(56, 189, 248, 0.28)",
    itemColorActiveCollapsed: "rgba(56, 189, 248, 0.2)",
    itemTextColorActive: "#38bdf8",
    itemTextColorActiveHover: "#7dd3fc",
    itemIconColorActive: "#38bdf8",
    itemIconColorActiveHover: "#7dd3fc",
  },
};

type NaiveTheme = typeof darkTheme;

/**
 * 主题状态与副作用的唯一来源：theme 为源，派生 Naive overrides，
 * 并通过一个 watcher 把 localStorage 与 <html data-theme> 同步出去。
 */
export function useTheme() {
  const theme: Ref<NaiveTheme | null> = ref(
    localStorage.getItem("theme") === "dark" ? darkTheme : null,
  );

  const isDark = computed(() => theme.value !== null);

  const overrides = computed(() =>
    theme.value === null
      ? themeOverrides
      : { ...themeOverrides, ...darkThemeOverrides },
  );

  function toggle() {
    theme.value = theme.value === null ? darkTheme : null;
  }

  watch(theme, (value) => {
    const mode = value === null ? "light" : "dark";
    localStorage.setItem("theme", mode);
    document.documentElement.setAttribute("data-theme", mode);
  });

  return { theme, isDark, overrides, toggle };
}
