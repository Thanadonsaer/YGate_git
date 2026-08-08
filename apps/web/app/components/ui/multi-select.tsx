"use client";

import * as React from "react";
import { MultiSelect as PrimeMultiSelect } from "primereact/multiselect";
import { ChevronDown, Search } from "lucide-react";
import { cn } from "../../lib/cn";

// `unit` and `tag` are what a telemetry point picker needs beyond a name: the
// engineering unit and the Modbus address (e.g. "FC3:40071"). Both optional, so
// plain {label, value} callers are unaffected.
type MultiSelectOption = { label: string; value: string; unit?: string; tag?: string };

function MultiSelect({
  value,
  onValueChange,
  options,
  placeholder,
  disabled,
  className,
  ariaLabel,
}: {
  value: string[];
  onValueChange: (values: string[]) => void;
  options: MultiSelectOption[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  ariaLabel?: string;
}) {
  // Only search the columns that exist, otherwise PrimeReact matches the
  // literal string "undefined" on options that have no unit or tag.
  const filterBy = options.some((option) => option.unit || option.tag) ? "label,unit,tag" : "label";
  return (
    <PrimeMultiSelect
      value={value}
      onChange={(event) => onValueChange((event.value as string[]) ?? [])}
      options={options}
      optionLabel="label"
      optionValue="value"
      disabled={disabled}
      placeholder={placeholder}
      aria-label={ariaLabel}
      filter
      filterBy={filterBy}
      filterPlaceholder="ค้นหา..."
      display="chip"
      itemTemplate={(option: MultiSelectOption) => (
        <>
          <span className="min-w-0 flex-1 truncate" title={option.label}>{option.label}</span>
          {option.unit && <span className="shrink-0 rounded bg-canvas px-1.5 py-0.5 text-[11px] font-bold text-ink-soft">{option.unit}</span>}
          {option.tag && <code className="shrink-0 text-[11px] text-ink-soft">{option.tag}</code>}
        </>
      )}
      dropdownIcon={<ChevronDown size={16} />}
      filterIcon={<Search size={14} />}
      unstyled
      className={cn(
        "flex min-h-10 w-full flex-wrap items-center gap-1 rounded-[var(--radius-sm)] border border-line bg-surface px-3 py-1.5 text-sm text-ink outline-none transition focus-within:border-focus focus-within:ring-4 focus-within:ring-focus/15 data-[p-disabled=true]:cursor-not-allowed data-[p-disabled=true]:opacity-48",
        className,
      )}
      panelClassName="rounded-[var(--radius-sm)] border border-line bg-surface shadow-[var(--shadow-lg)]"
      pt={{
        labelContainer: { className: "flex-1 min-w-0" },
        label: { className: "flex flex-wrap gap-1 text-left" },
        token: { className: "flex items-center gap-1 rounded-full bg-canvas px-2 py-0.5 text-xs font-bold text-ink" },
        trigger: { className: "ml-auto flex shrink-0 items-center text-ink-soft" },
        header: { className: "flex items-center gap-2 border-b border-line p-2" },
        filterContainer: { className: "relative flex-1" },
        filterInput: { className: "h-9 w-full rounded-[var(--radius-sm)] border border-line bg-canvas pl-8 pr-2 text-sm outline-none focus:border-focus" },
        filterIcon: { className: "pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-ink-soft" },
        headerCheckboxContainer: { className: "hidden" },
        // overscroll-contain stops the panel handing its leftover scroll to the
        // page once the list hits an end -- otherwise scrolling the options
        // scrolls the whole view out from under the open dropdown.
        wrapper: { className: "max-h-72 overflow-y-auto overscroll-contain" },
        list: { className: "flex flex-col gap-0.5 p-1" },
        emptyMessage: { className: "px-3 py-2 text-sm text-ink-soft" },
        item: (options: { context: { focused: boolean; selected: boolean; disabled: boolean } }) => ({
          className: cn(
            "flex w-full cursor-pointer select-none items-center gap-2 rounded-[var(--radius-sm)] py-2 px-2 text-sm text-ink outline-none",
            options.context.focused && "bg-canvas",
            options.context.selected && "font-bold",
            options.context.disabled && "pointer-events-none opacity-48",
          ),
        }),
        checkboxContainer: { className: "flex-none" },
      }}
    />
  );
}

export { MultiSelect };
export type { MultiSelectOption };
