<script setup lang="ts" generic="T">
import { NEmpty, NSpin } from "naive-ui";
import { t } from "../i18n";

/**
 * 手机端表格的卡片列表替身：每行一张卡片，内容由 item 插槽排版。
 * 桌面端仍使用 n-data-table；这里只负责容器、加载态、空态与卡片外观，
 * 单元格内容请配合 TableCell 复用表格列的 render，避免重复实现。
 */
defineProps<{
  rows: T[];
  rowKey: (row: T) => string | number;
  loading?: boolean;
}>();

defineSlots<{
  item(props: { row: T; index: number }): unknown;
}>();
</script>

<template>
  <n-spin :show="loading">
    <div v-if="rows.length" class="mcl" role="list">
      <article
        v-for="(row, index) in rows"
        :key="rowKey(row)"
        class="mcl__item"
        role="listitem"
      >
        <slot name="item" :row="row" :index="index" />
      </article>
    </div>
    <n-empty v-else :description="t('common.noData')" class="mcl__empty" />
  </n-spin>
</template>

<style scoped>
.mcl {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.mcl__item {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  min-width: 0;
  background: var(--comic-surface);
  border: var(--comic-border) solid var(--comic-ink);
  border-radius: var(--comic-radius);
  box-shadow: var(--comic-shadow-sm);
}

.mcl__empty {
  padding: 24px 0;
}
</style>

<!-- 非 scoped：卡片内容由各视图的插槽渲染，其 scope 属于调用方，故共享类统一加 mcl- 前缀放在这里 -->
<style>
.mcl-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
}

.mcl-grow {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}

.mcl-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 14px;
  font-size: 12px;
  opacity: 0.8;
}

.mcl-kv {
  display: inline-flex;
  align-items: flex-start;
  gap: 4px;
  min-width: 0;
  overflow-wrap: anywhere;
}

.mcl-k {
  flex-shrink: 0;
  white-space: nowrap;
  opacity: 0.7;
}

.mcl-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

/* 卡片内的 tiny 图标按钮（如复制密钥，原 22px）放大到 32px。
   Naive 渲染的按钮只带 n-button--<size>-type 这类尺寸类名，没有 circle 之类的修饰类，
   故用尺寸类命中；须排在操作区规则之前，让后者（38px）覆盖 */
.mcl__item .n-button--tiny-type {
  width: 32px !important;
  height: 32px !important;
}

/* 手机端操作区点击目标不小于 38px（Naive 的 small 按钮只有 28px） */
.mcl-actions .n-button {
  width: auto;
  min-width: 38px;
  height: 38px !important;
}

.mcl-pager {
  display: flex;
  justify-content: center;
  margin-top: 12px;
}

/* 外层表格卡片内已是卡片列表时，去掉外层的粗边框/阴影/内边距，避免卡片套卡片 */
[data-theme] .n-card.table-card:has(.mcl) {
  background: transparent;
  border: none;
  box-shadow: none;
}

[data-theme] .n-card.table-card:has(.mcl) > .n-card__content {
  padding: 0;
}
</style>
