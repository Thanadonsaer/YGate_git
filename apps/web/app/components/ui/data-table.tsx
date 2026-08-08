"use client";

import * as React from "react";
import { DataTable as PrimeDataTable, type DataTableProps, type DataTableValueArray, type SortOrder } from "primereact/datatable";
import { Column, type ColumnProps } from "primereact/column";
import { ArrowDown, ArrowUp, ChevronsUpDown } from "lucide-react";
import { cn } from "../../lib/cn";

// The app runs PrimeReact unstyled (see layout.tsx), so every visual comes from
// pt. Keeping the skin here beats repeating ~40 lines of pt at each of the 13
// table call sites. The look matches the CSS grid tables it replaces: white
// surface, 1px line border, muted 11px sticky header, 13px body text.
function tableClasses(striped?: boolean) {
  return {
    root: { className: "w-full max-w-full border border-line bg-surface" },
    wrapper: { className: "w-full max-w-full overflow-x-auto" },
    table: { className: "w-full border-collapse text-[13px]" },
    thead: { className: "" },
    headerRow: { className: "" },
    bodyRow: (options?: { context?: { index?: number; selected?: boolean } }) => ({
      className: cn(
        "border-b border-line last:border-b-0",
        striped && (options?.context?.index ?? 0) % 2 === 1 && "bg-canvas/40",
        options?.context?.selected && "bg-canvas",
      ),
    }),
    paginator: {
      root: { className: "flex flex-wrap items-center justify-end gap-1 border-t border-line bg-surface px-3 py-2 text-xs" },
      pageButton: (options?: { context?: { active?: boolean } }) => ({
        className: cn(
          "inline-grid h-8 min-w-8 place-items-center rounded-[var(--radius-sm)] px-2 font-bold transition",
          options?.context?.active ? "bg-brand text-white" : "text-ink-soft hover:bg-canvas",
        ),
      }),
      firstPageButton: { className: "inline-grid h-8 w-8 place-items-center rounded-[var(--radius-sm)] text-ink-soft transition hover:bg-canvas disabled:opacity-40" },
      previousPageButton: { className: "inline-grid h-8 w-8 place-items-center rounded-[var(--radius-sm)] text-ink-soft transition hover:bg-canvas disabled:opacity-40" },
      nextPageButton: { className: "inline-grid h-8 w-8 place-items-center rounded-[var(--radius-sm)] text-ink-soft transition hover:bg-canvas disabled:opacity-40" },
      lastPageButton: { className: "inline-grid h-8 w-8 place-items-center rounded-[var(--radius-sm)] text-ink-soft transition hover:bg-canvas disabled:opacity-40" },
      current: { className: "mr-auto text-ink-soft" },
    },
  };
}

const columnPt = {
  // The background lives on the cell, not the row: a sticky `th` paints its own
  // background, and a `tr` background does not show through underneath it.
  headerCell: { className: "sticky top-0 z-[5] whitespace-nowrap bg-[#f7f9fa] px-4 py-2.5 text-left align-middle text-[11px] font-bold text-ink-soft" },
  headerContent: { className: "flex items-center gap-1.5" },
  headerTitle: { className: "min-w-0 truncate" },
  bodyCell: { className: "min-w-0 px-4 py-3 align-middle" },
  sort: { className: "inline-flex shrink-0 items-center text-ink-soft" },
};

/**
 * Sort indicator. PrimeReact passes this column's current order:
 * 1 = ascending, -1 = descending, 0/undefined = unsorted.
 */
function SortIcon({ sortOrder }: { sortOrder?: SortOrder }) {
  if (sortOrder === 1) return <ArrowUp size={13} />;
  if (sortOrder === -1) return <ArrowDown size={13} />;
  return <ChevronsUpDown size={13} className="opacity-45" />;
}

function DataTable<T extends DataTableValueArray>({ striped, children, ...props }: DataTableProps<T> & { striped?: boolean }) {
  // DataTable never renders its children -- it reads `child.props` straight off
  // the elements. So a wrapper component around Column silently loses whatever
  // it would add in its own function body: that `pt` never reaches the table,
  // leaving `th`/`td` with browser defaults (centred headers, zero padding).
  // Clone the real Column elements instead.
  const columns = React.Children.map(children, (child) => {
    if (!React.isValidElement<ColumnProps>(child)) return child;
    return React.cloneElement(child, {
      pt: { ...columnPt, ...(child.props.pt ?? {}) },
      // A header-less column is the row-actions column; a blank `th` leaves
      // screen readers announcing an unnamed column, so keep the name hidden.
      header: child.props.header || <span className="sr-only">คำสั่ง</span>,
    });
  });

  return (
    <PrimeDataTable<T>
      // A sortable header is only discoverable if it looks clickable.
      sortIcon={(options) => <SortIcon sortOrder={options.sortOrder} />}
      emptyMessage={<div className="table-state">ไม่มีข้อมูล</div>}
      unstyled
      pt={tableClasses(striped)}
      {...props}
    >
      {columns}
    </PrimeDataTable>
  );
}

export { DataTable, Column as TableColumn };
