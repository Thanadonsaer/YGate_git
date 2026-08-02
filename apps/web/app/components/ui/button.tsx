"use client";

import { Button as PrimeButton, type ButtonProps as PrimeButtonProps } from "primereact/button";
import { cn } from "../../lib/cn";

type Variant = "primary" | "secondary" | "icon" | "text";

const base =
  "inline-flex items-center justify-center gap-1.5 font-bold transition disabled:cursor-not-allowed disabled:opacity-48 focus:outline-none focus-visible:ring-2 focus-visible:ring-focus";

const variantClass: Record<Variant, string> = {
  primary: "rounded-[var(--radius-md)] bg-brand px-4 py-2 text-sm text-white shadow-[var(--shadow-sm)] hover:brightness-110",
  secondary: "rounded-[var(--radius-md)] border border-line bg-surface px-4 py-2 text-sm text-ink hover:bg-canvas",
  icon: "rounded-[var(--radius-sm)] p-2 text-ink-soft hover:bg-canvas",
  text: "text-sm text-brand hover:underline",
};

type Props = Omit<PrimeButtonProps, "variant"> & {
  variant?: Variant;
  compact?: boolean;
  danger?: boolean;
  iconOnly?: boolean;
};

export function Button(props: Props) {
  const { variant = "primary", compact, danger, iconOnly, className, ...rest } = props;
  const v = variant as Variant;
  return (
    <PrimeButton
      unstyled
      className={cn(
        base,
        variantClass[v],
        compact && "px-2.5 py-1.5 text-xs",
        iconOnly && "px-2 py-2",
        danger && "text-[var(--danger)]",
        className,
      )}
      {...rest}
    />
  );
}
