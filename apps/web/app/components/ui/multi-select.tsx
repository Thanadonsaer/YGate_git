"use client";

import * as React from "react";
import { MultiSelect as PrimeMultiSelect } from "primereact/multiselect";
import { ChevronDown, Search } from "lucide-react";
import { cn } from "../../lib/cn";

type Option = { label: string; value: string };

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
  options: Option[];
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  ariaLabel?: string;
}) {
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
      filterPlaceholder="ค้นหา..."
      display="chip"
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
        wrapper: { className: "max-h-72 overflow-y-auto" },
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
