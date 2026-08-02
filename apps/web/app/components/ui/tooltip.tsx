"use client";

import * as React from "react";
import { cn } from "../../lib/cn";

type TooltipContextValue = { open: boolean; setOpen: (open: boolean) => void };
const TooltipContext = React.createContext<TooltipContextValue | null>(null);

function Tooltip({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = React.useState(false);
  return (
    <TooltipContext.Provider value={{ open, setOpen }}>
      <span className="relative inline-block">{children}</span>
    </TooltipContext.Provider>
  );
}

function TooltipTrigger({ children }: { asChild?: boolean; children: React.ReactElement }) {
  const ctx = React.useContext(TooltipContext);
  if (!ctx) throw new Error("TooltipTrigger must be used inside Tooltip");
  return React.cloneElement(children, {
    onMouseEnter: () => ctx.setOpen(true),
    onMouseLeave: () => ctx.setOpen(false),
    onFocus: () => ctx.setOpen(true),
    onBlur: () => ctx.setOpen(false),
  } as React.HTMLAttributes<HTMLElement>);
}

function TooltipContent({ className, children }: { className?: string; children: React.ReactNode }) {
  const ctx = React.useContext(TooltipContext);
  if (!ctx?.open) return null;
  return (
    <span
      role="tooltip"
      className={cn(
        "absolute left-0 top-full z-50 mt-1 max-w-64 rounded-[var(--radius-sm)] bg-ink px-2.5 py-1.5 text-xs font-semibold text-surface shadow-[var(--shadow-sm)]",
        className,
      )}
    >
      {children}
    </span>
  );
}

export { Tooltip, TooltipTrigger, TooltipContent };
