"use client";

import * as React from "react";
import { Toast } from "primereact/toast";
import { cn } from "../../lib/cn";

let activeToast: Toast | null = null;

// unstyled mode (see PrimeReactProvider in layout.tsx) strips PrimeReact's
// entire default CSS for Toast, including the position:fixed placement and
// the per-severity colors -- pt below has to supply both from scratch, the
// same way select.tsx/tabs.tsx do for their own PrimeReact primitives.
const severityClass: Record<string, string> = {
  success: "border-[color-mix(in_srgb,var(--success)_35%,var(--line))] bg-[color-mix(in_srgb,var(--success)_10%,var(--surface))]",
  error: "border-[color-mix(in_srgb,var(--danger)_35%,var(--line))] bg-[color-mix(in_srgb,var(--danger)_10%,var(--surface))]",
  warn: "border-[color-mix(in_srgb,var(--warning)_35%,var(--line))] bg-[color-mix(in_srgb,var(--warning)_10%,var(--surface))]",
  info: "border-line bg-surface",
};

const iconClass: Record<string, string> = {
  success: "text-[var(--success)]",
  error: "text-[var(--danger)]",
  warn: "text-[var(--warning)]",
  info: "text-[var(--focus)]",
};

function Toaster() {
  const ref = React.useRef<Toast>(null);
  React.useEffect(() => {
    activeToast = ref.current;
    return () => {
      activeToast = null;
    };
  }, []);
  return (
    <Toast
      ref={ref}
      position="bottom-right"
      pt={{
        root: { className: "fixed bottom-5 right-5 z-[9999] flex w-full max-w-sm flex-col gap-2" },
        message: (options: { message?: { message?: { severity?: string } } }) => ({
          className: cn(
            "rounded-[var(--radius-md)] border px-4 py-3 text-sm text-ink shadow-[var(--shadow-lg)]",
            severityClass[options.message?.message?.severity ?? "info"],
          ),
        }),
        content: { className: "flex items-start gap-3" },
        icon: (options: { message?: { message?: { severity?: string } } }) => ({
          className: cn("mt-0.5 h-5 w-5 shrink-0", iconClass[options.message?.message?.severity ?? "info"]),
        }),
        text: { className: "min-w-0 flex-1" },
        summary: { className: "block font-semibold" },
        detail: { className: "mt-1 block text-xs text-ink-soft" },
        closeButton: { className: "ml-1 shrink-0 rounded-[var(--radius-sm)] p-1 text-ink-soft outline-none hover:bg-canvas" },
        closeButtonIcon: { className: "h-4 w-4" },
      }}
    />
  );
}

export const toast = {
  success(message: string) {
    activeToast?.show({ severity: "success", summary: message, life: 3500 });
  },
  error(message: string) {
    activeToast?.show({ severity: "error", summary: message, life: 5000 });
  },
};

export { Toaster };
