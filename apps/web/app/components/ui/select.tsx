"use client";

import * as React from "react";
import { Dropdown } from "primereact/dropdown";
import { Check } from "lucide-react";
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
      value={value}
      onChange={(event) => onValueChange?.(event.value as string)}
      disabled={disabled}
      options={options}
      optionLabel="label"
      optionValue="value"
      optionDisabled="disabled"
      placeholder={values[0]?.props.placeholder}
      ariaLabel={triggers[0]?.props["aria-label"]}
      itemTemplate={(option: Option) => (
        <span className="relative flex w-full items-center gap-2 pl-6">
          {option.value === value && <Check size={14} className="absolute left-0 text-brand" />}
          {option.label}
        </span>
      )}
      unstyled
      className={cn("flex h-10 w-full items-center gap-2 rounded-[var(--radius-sm)] border border-line bg-surface px-3 text-sm text-ink", triggers[0]?.props.className)}
      panelClassName="rounded-[var(--radius-sm)] border border-line bg-surface shadow-[var(--shadow-lg)]"
    />
  );
}

export { Select, SelectValue, SelectTrigger, SelectContent, SelectItem };
