"use client";

import * as React from "react";
import { MultiSelect as PrimeMultiSelect } from "primereact/multiselect";
import { ChevronDown, Search } from "lucide-react";
import { checkboxPt } from "./form";
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
  // Each column is checked on its own: PrimeReact stringifies a missing field,
  // so naming "unit" while no option has one makes every option match a search
  // for "und". Device pickers pass a tag with no unit, parameter pickers pass
  // both, and a plain {label,value} caller searches label alone.
  const filterBy = ["label", options.some((option) => option.unit) && "unit", options.some((option) => option.tag) && "tag"]
    .filter(Boolean)
    .join(",");
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
      // The template's output lands inside a bare <span> PrimeReact renders
      // with no passthrough hook of its own, so the flex row has to come from
      // the template itself. Returning a fragment left label/unit/tag as inline
      // children of that span: the label's flex-1/truncate did nothing and a
      // long parameter name pushed the unit and address chips off the row.
      itemTemplate={(option: MultiSelectOption) => (
        <span className="flex min-w-0 flex-1 items-center gap-2">
          <span className="min-w-0 flex-1 truncate" title={option.label}>{option.label}</span>
          {option.unit && <span className="shrink-0 rounded bg-canvas px-1.5 py-0.5 text-[11px] font-bold text-ink-soft">{option.unit}</span>}
          {option.tag && <code className="shrink-0 text-[11px] text-ink-soft">{option.tag}</code>}
        </span>
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
            // itemTemplate's output sits in a bare <span> that PrimeReact gives
            // no passthrough hook, so it is reached from the row instead --
            // without this it sizes to content and long labels overflow the panel.
            "[&>span]:min-w-0 [&>span]:flex-1",
            options.context.focused && "bg-canvas",
            options.context.selected && "font-bold",
            options.context.disabled && "pointer-events-none opacity-48",
          ),
        }),
        checkboxContainer: { className: "flex-none" },
        // The per-option checkbox is a nested PrimeReact <Checkbox> that gets
        // whatever `pt.checkbox` holds. Unstyled, so with nothing here it drew
        // an unpainted zero-size div next to every option; this is the same
        // skin the standalone Checkbox uses.
        checkbox: checkboxPt(),
      }}
    />
  );
}

export { MultiSelect };
export type { MultiSelectOption };
