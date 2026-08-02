"use client";

import * as React from "react";
import { Dialog as PrimeDialog } from "primereact/dialog";
import type { DialogRootChangeEvent } from "@primereact/types/primitive/dialog";
import { X } from "lucide-react";
import { cn } from "../../lib/cn";

type DialogContextValue = { onOpenChange?: (open: boolean) => void };
const DialogContext = React.createContext<DialogContextValue>({});

function Dialog({
  open,
  onOpenChange,
  children,
}: {
  open: boolean;
  onOpenChange?: (open: boolean) => void;
  children: React.ReactNode;
}) {
  if (!open) return null;
  return <DialogContext.Provider value={{ onOpenChange }}>{children}</DialogContext.Provider>;
}

function DialogContent({
  className,
  children,
  showClose = true,
}: {
  className?: string;
  children: React.ReactNode;
  showClose?: boolean;
}) {
  const { onOpenChange } = React.useContext(DialogContext);
  return (
    <PrimeDialog.Root
      open
      unstyled
      modal
      dismissable={false}
      draggable={false}
      blockScroll
      onOpenChange={(event: DialogRootChangeEvent) => {
        if (!event.value) onOpenChange?.(false);
      }}
    >
      <PrimeDialog.Portal>
        <PrimeDialog.Backdrop unstyled className="fixed inset-0 z-50 bg-ink/55" />
        <PrimeDialog.Positioner unstyled className="fixed inset-0 z-50 flex items-center justify-center p-4">
          <PrimeDialog.Popup
            unstyled
            className={cn(
              "relative w-full max-w-lg max-h-[calc(100vh-2rem)] overflow-y-auto rounded-[var(--radius-lg)] border border-line bg-surface shadow-[var(--shadow-lg)]",
              className,
            )}
          >
            {children}
            {showClose && (
              <button
                type="button"
                onClick={() => onOpenChange?.(false)}
                className="absolute right-4 top-4 rounded-[var(--radius-sm)] p-1.5 text-ink-soft transition hover:bg-canvas focus:outline-none focus-visible:ring-2 focus-visible:ring-focus"
                aria-label="ปิด"
                title="ปิด"
              >
                <X size={18} />
              </button>
            )}
          </PrimeDialog.Popup>
        </PrimeDialog.Positioner>
      </PrimeDialog.Portal>
    </PrimeDialog.Root>
  );
}

function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("flex items-center justify-between gap-4 border-b border-line px-5 py-4", className)} {...props} />;
}

function DialogTitle({ className, ...props }: React.ComponentProps<"h2">) {
  return <h2 className={cn("font-display text-xl font-extrabold text-ink", className)} {...props} />;
}

function DialogDescription({ className, ...props }: React.ComponentProps<"p">) {
  return <p className={cn("text-xs font-extrabold uppercase text-ink-soft", className)} {...props} />;
}

function DialogBody({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("p-5", className)} {...props} />;
}

export { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody };
