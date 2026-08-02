"use client";

import * as React from "react";
import { Select as PrimeSelect } from "primereact/select";
import type { SelectValueChangeEvent } from "@primereact/types/primitive/select";
import { Check, ChevronDown } from "lucide-react";
import { collectByType } from "./collect-children";
import { cn } from "../../lib/cn";

function SelectValue(_props: { placeholder?: string }) {
  return null;
}

function SelectTrigger({ children }: React.ComponentProps<"button"> & { children?: React.ReactNode }) {
  return <>{children}</>;
}

function SelectContent({ children }: { children?: React.ReactNode }) {
  return <>{children}</>;
}

function SelectItem({ children }: { value: string; disabled?: boolean; children?: React.ReactNode }) {
  return <>{children}</>;
}

// SelectItem children are often interpolated JSX (e.g. `{a} / {b}`), which React
// represents as an array of nodes rather than a single string. Select.Value's
// fallback renderer joins array labels with ", " (its multi-select codepath), so an
// un-flattened array label would render as "a, /, b" instead of "a / b". Flatten to
// plain text so the option label always behaves as a single string.
function labelText(node: React.ReactNode): string {
  if (Array.isArray(node)) return node.map(labelText).join("");
  return node === null || node === undefined || typeof node === "boolean" ? "" : String(node);
}

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
  const triggers = collectByType<React.ComponentProps<"button">>(children, SelectTrigger);
  const options = items.map((item) => ({ label: labelText(item.props.children), value: item.props.value, disabled: item.props.disabled }));
  const { className: triggerClassName, children: _triggerChildren, ...triggerRest } = triggers[0]?.props ?? {};

  return (
    <PrimeSelect.Root
      value={value}
      onValueChange={(event: SelectValueChangeEvent) => onValueChange?.(event.value as string)}
      disabled={disabled}
      options={options}
      optionLabel="label"
      optionValue="value"
      optionDisabled="disabled"
      unstyled
      pt={{
        option:
          "relative flex w-full cursor-pointer select-none items-center gap-2 rounded-[var(--radius-sm)] py-2 pl-8 pr-2 text-sm text-ink outline-none data-[focused]:bg-canvas data-[disabled]:pointer-events-none data-[disabled]:opacity-48",
      }}
    >
      <PrimeSelect.Trigger
        {...triggerRest}
        type="button"
        unstyled
        className={cn(
          "flex h-10 w-full items-center justify-between gap-2 rounded-[var(--radius-sm)] border border-line bg-surface px-3 text-sm text-ink outline-none transition focus:border-focus focus:ring-4 focus:ring-focus/15 disabled:cursor-not-allowed disabled:opacity-48",
          triggerClassName,
        )}
      >
        <PrimeSelect.Value unstyled placeholder={values[0]?.props.placeholder} className="truncate text-left data-[placeholder]:text-ink-soft" />
        <PrimeSelect.Indicator unstyled className="shrink-0 text-ink-soft">
          <ChevronDown size={16} />
        </PrimeSelect.Indicator>
      </PrimeSelect.Trigger>
      <PrimeSelect.Portal>
        <PrimeSelect.Positioner unstyled className="z-50 min-w-[8rem] w-[var(--px-positioner-anchor-width)]">
          <PrimeSelect.Popup
            unstyled
            className="max-h-96 overflow-y-auto rounded-[var(--radius-sm)] border border-line bg-surface p-1 shadow-[var(--shadow-lg)]"
          >
            <PrimeSelect.List unstyled>
              {options.map((option, index) => (
                <PrimeSelect.Option key={String(option.value)} index={index} unstyled>
                  <span className="absolute left-2 flex size-3.5 items-center justify-center">
                    <PrimeSelect.OptionIndicator unstyled className="hidden items-center justify-center data-[selected]:flex">
                      <Check size={14} className="text-brand" />
                    </PrimeSelect.OptionIndicator>
                  </span>
                  {option.label}
                </PrimeSelect.Option>
              ))}
            </PrimeSelect.List>
          </PrimeSelect.Popup>
        </PrimeSelect.Positioner>
      </PrimeSelect.Portal>
    </PrimeSelect.Root>
  );
}

export { Select, SelectValue, SelectTrigger, SelectContent, SelectItem };
