import { createApp } from 'vue'
import App from './App.vue'
import { applyLocaleToDocument } from "./i18n";
import "./styles/comic.css";

applyLocaleToDocument();
// 在挂载前写入 data-theme，供 comic.css 区分明暗并避免主题闪烁(FOUC)
document.documentElement.setAttribute(
  "data-theme",
  localStorage.getItem("theme") === "dark" ? "dark" : "light",
);
createApp(App).mount('#app')
