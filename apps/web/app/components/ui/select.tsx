"use client";

import * as React from "react";
import { Dropdown } from "primereact/dropdown";
import { Check, ChevronDown, Search } from "lucide-react";
import { collectByType } from "./collect-children";
import { cn } from "../../lib/cn";

function SelectValue(_props: { placeholder?: string }) {
  return null;
}

function SelectTrigger({ children }: { className?: string; "aria-label"?: string; children?: React.ReactNode }) {
  return <>{children}</>;
}

function SelectContent({ children }: { children?: React.ReactNode }) {
  return <>{children}</>;
}

function SelectItem({ children }: { value: string; disabled?: boolean; children?: React.ReactNode }) {
  return <>{children}</>;
}

// SelectItem children are often interpolated JSX (e.g. `{a} / {b}`), which React
// represents as an array of nodes rather than a single string. Flatten to plain
// text so the option label always behaves as a single string for Dropdown.
function labelText(node: React.ReactNode): string {
  if (Array.isArray(node)) return node.map(labelText).join("");
  return node === null || node === undefined || typeof node === "boolean" ? "" : String(node);
}

type Option = { label: string; value: string; disabled?: boolean };

function Select({
  value,
  onValueChange,
  disabled,
  children,
}: {
  value?: string;
  onValueChange?: (value: string) => void;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  const items = collectByType<{ value: string; disabled?: boolean; children?: React.ReactNode }>(children, SelectItem);
  const values = collectByType<{ placeholder?: string }>(children, SelectValue);
  const triggers = collectByType<{ className?: string; "aria-label"?: string }>(children, SelectTrigger);
  const options: Option[] = items.map((item) => ({ label: labelText(item.props.children), value: item.props.value, disabled: item.props.disabled }));

  return (
    <Dropdown
      // A search box over four fixed choices (Shape, Timezone, Display...) is
      // just noise; it earns its place once the list stops fitting on screen.
      filter={options.length > 7}
      filterBy="label"
      filterPlaceholder="ค้นหา..."
      resetFilterOnHide
      filterIcon={<Search size={14} />}
      value={value}
      onChange={(event) => onValueChange?.(event.value as string)}
      disabled={disabled}
      options={options}
      optionLabel="label"
      optionValue="value"
      optionDisabled="disabled"
      placeholder={values[0]?.props.placeholder}
      ariaLabel={triggers[0]?.props["aria-label"]}
      dropdownIcon={<ChevronDown size={16} />}
      itemTemplate={(option: Option) => (
        <span className="relative flex w-full items-center gap-2 pl-6">
          {option.value === value && <Check size={14} className="absolute left-0 text-brand" />}
          {option.label}
        </span>
      )}
      unstyled
      className={cn("flex h-10 w-full items-center gap-2 rounded-[var(--radius-sm)] border border-line bg-surface px-3 text-sm text-ink outline-none transition focus-within:border-focus focus-within:ring-4 focus-within:ring-focus/15 data-[p-disabled=true]:cursor-not-allowed data-[p-disabled=true]:opacity-48", triggers[0]?.props.className)}
      panelClassName="rounded-[var(--radius-sm)] border border-line bg-surface shadow-[var(--shadow-lg)]"
      pt={{
        input: { className: "flex-1 min-w-0 truncate text-left outline-none" },
        trigger: { className: "ml-auto flex shrink-0 items-center text-ink-soft" },
        header: { className: "flex items-center gap-2 border-b border-line p-2" },
        filterContainer: { className: "relative flex-1" },
        filterInput: { className: "h-9 w-full rounded-[var(--radius-sm)] border border-line bg-canvas pl-8 pr-2 text-sm outline-none focus:border-focus" },
        filterIcon: { className: "pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-ink-soft" },
        // overscroll-contain stops the panel handing its leftover scroll to the
        // page once the list hits an end -- otherwise scrolling the options
        // scrolls the whole view out from under the open dropdown.
        wrapper: { className: "max-h-96 overflow-y-auto overscroll-contain" },
        list: { className: "flex flex-col gap-0.5 p-1" },
        emptyMessage: { className: "px-3 py-2 text-sm text-ink-soft" },
        item: (options: { context: { focused: boolean; disabled: boolean } }) => ({
          className: cn(
            "relative flex w-full cursor-pointer select-none items-center rounded-[var(--radius-sm)] py-2 pr-2 text-sm text-ink outline-none",
            options.context.focused && "bg-canvas",
            options.context.disabled && "pointer-events-none opacity-48",
          ),
        }),
      }}
    />
  );
}

export { Select, SelectValue, SelectTrigger, SelectContent, SelectItem };
