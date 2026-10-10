import { ref } from "vue";
import { api } from "../api/client";
import type { KeyItem, PortalMe, PortalOverview, TimeSeries } from "../types";

/**
 * 捐赠者门户的共享读状态与加载动作。变更类操作（增 Key、重置密钥）由对应的
 * 自包含组件就地发起（结果是局部的），删除等简单动作经事件回到容器调用这里。
 */
export function useDonorPortal() {
  const me = ref<PortalMe | null>(null);
  const overview = ref<PortalOverview | null>(null);
  const timeseries = ref<TimeSeries | null>(null);
  const keys = ref<KeyItem[]>([]);

  const loadingOverview = ref(false);
  const loadingChart = ref(false);
  const loadingKeys = ref(false);

  async function loadMe() {
    const { data } = await api.get<PortalMe>("/api/portal/me");
    me.value = data;
  }

  async function loadOverview() {
    loadingOverview.value = true;
    try {
      const { data } = await api.get<PortalOverview>("/api/portal/overview");
      overview.value = data;
    } finally {
      loadingOverview.value = false;
    }
  }

  async function loadTimeseries(granularity: "hour" | "day" | "month") {
    loadingChart.value = true;
    try {
      const { data } = await api.get<TimeSeries>("/api/portal/timeseries", {
        params: { granularity },
      });
      timeseries.value = data;
    } finally {
      loadingChart.value = false;
    }
  }

  async function loadKeys() {
    loadingKeys.value = true;
    try {
      const { data } = await api.get<{ items: KeyItem[] }>("/api/portal/keys");
      keys.value = data.items;
    } finally {
      loadingKeys.value = false;
    }
  }

  async function deleteKey(id: number) {
    await api.delete(`/api/portal/keys/${id}`);
  }

  return {
    me,
    overview,
    timeseries,
    keys,
    loadingOverview,
    loadingChart,
    loadingKeys,
    loadMe,
    loadOverview,
    loadTimeseries,
    loadKeys,
    deleteKey,
  };
}
