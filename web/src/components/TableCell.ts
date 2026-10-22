import type { FunctionalComponent, VNodeChild } from "vue";
import type { DataTableColumn } from "naive-ui";

type AnyColumn = DataTableColumn<any>;

interface Props {
  column?: AnyColumn;
  row: unknown;
  index?: number;
}

/**
 * 在卡片布局里渲染某个表格列的单元格：优先复用列的 render（状态标签、进度条、
 * 带确认框的操作按钮等），没有 render 的列退化为直接输出 row[key]。
 */
export const TableCell: FunctionalComponent<Props> = (props) => {
  const column = props.column as
    | (AnyColumn & { render?: (row: unknown, index: number) => VNodeChild })
    | undefined;
  if (!column) return null;
  if (column.render) {
    return column.render(props.row, props.index ?? 0) as any;
  }
  const key = (column as { key?: string | number }).key;
  const value = key === undefined ? "" : (props.row as Record<string, unknown>)[key];
  return value === undefined || value === null ? "" : String(value);
};
TableCell.props = ["column", "row", "index"];

/** 列标题（title 可能是函数，以便随语言切换）。 */
export function columnTitle(column: AnyColumn | undefined): string {
  const title = (column as { title?: unknown } | undefined)?.title;
  if (typeof title === "function") return String((title as () => unknown)() ?? "");
  return title === undefined || title === null ? "" : String(title);
}

/** 以 key 为索引的列字典，便于在模板里按名取列。 */
export function columnsByKey(columns: AnyColumn[]): Record<string, AnyColumn> {
  const out: Record<string, AnyColumn> = {};
  for (const column of columns) {
    const key = (column as { key?: string | number }).key;
    if (key !== undefined) out[String(key)] = column;
  }
  return out;
}
