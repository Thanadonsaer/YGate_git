"use client";

import * as React from "react";
import { cn } from "../../lib/cn";

type PopoverContextValue = { open: boolean; setOpen: (open: boolean) => void };
const PopoverContext = React.createContext<PopoverContextValue | null>(null);

function Popover({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = React.useState(false);
  const rootRef = React.useRef<HTMLSpanElement>(null);
  React.useEffect(() => {
    if (!open) return;
    function handleClickOutside(event: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(event.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [open]);
  return (
    <PopoverContext.Provider value={{ open, setOpen }}>
      <span ref={rootRef} className="relative inline-block">
        {children}
      </span>
    </PopoverContext.Provider>
  );
}

function PopoverTrigger({ children }: { asChild?: boolean; children: React.ReactElement }) {
  const ctx = React.useContext(PopoverContext);
  if (!ctx) throw new Error("PopoverTrigger must be used inside Popover");
  return React.cloneElement(children, {
    onClick: () => ctx.setOpen(!ctx.open),
  } as React.HTMLAttributes<HTMLElement>);
}

function PopoverContent({
  className,
  align = "center",
  children,
}: {
  className?: string;
  align?: "start" | "center" | "end";
  children: React.ReactNode;
}) {
  const ctx = React.useContext(PopoverContext);
  if (!ctx?.open) return null;
  const alignClass = align === "start" ? "left-0" : align === "end" ? "right-0" : "left-1/2 -translate-x-1/2";
  return (
    <div
      className={cn(
        "absolute top-full z-50 mt-2 w-72 rounded-[var(--radius-md)] border border-line bg-surface p-4 text-sm text-ink shadow-[var(--shadow-lg)]",
        alignClass,
        className,
      )}
    >
      {children}
    </div>
  );
}

export { Popover, PopoverTrigger, PopoverContent };
