import { onBeforeUnmount, onMounted, ref, type Ref } from "vue";

/**
 * 响应式媒体查询：返回一个跟随 matchMedia 变化的只读 ref。
 * 初始值在 setup 期即取到（避免首帧误判），监听在挂载/卸载时管理。
 */
export function useMediaQuery(query: string): Ref<boolean> {
  const mql =
    typeof window !== "undefined" && "matchMedia" in window
      ? window.matchMedia(query)
      : null;
  const matches = ref(mql?.matches ?? false);

  function update(event: MediaQueryListEvent) {
    matches.value = event.matches;
  }

  onMounted(() => mql?.addEventListener("change", update));
  onBeforeUnmount(() => mql?.removeEventListener("change", update));

  return matches;
}
