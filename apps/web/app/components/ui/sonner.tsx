// apps/web/app/components/ui/sonner.tsx
"use client";

import * as React from "react";
import { Toaster as PrimeToaster, toast as primeToast, useToasterContext } from "primereact/toaster";
import { Toast as PrimeToast } from "primereact/toast";
import { cn } from "../../lib/cn";

// The installed primereact@^11 "primereact/toast" + "primereact/toaster" are headless
// compound-component trees (Toaster.Root/Portal/Region + Toast.Root/Title/...), not the
// old ref + `.show()` single component the plan brief assumed. Their imperative `toast()`
// helper (exported from "primereact/toaster") is itself modeled directly on Sonner
// (see node_modules/@primereact/headless/toast/useToast.d.ts docblock), so it maps onto
// the same module-level toast.success(message)/toast.error(message) call pattern.

function ToastItems() {
  const toaster = useToasterContext();
  const toasts = toaster?.toasts ?? [];

  return (
    <>
      {toasts.map((item) => (
        <PrimeToast.Root
          key={item.id}
          toast={item}
          unstyled
          className={cn(
            "rounded-[var(--radius-md)] border border-line bg-surface px-4 py-3 text-sm text-ink shadow-[var(--shadow-lg)]",
            item.severity === "success" && "border-success/30",
            item.severity === "error" && "border-danger/30",
          )}
        >
          <PrimeToast.Title unstyled className="font-bold" />
        </PrimeToast.Root>
      ))}
    </>
  );
}

function Toaster() {
  return (
    <PrimeToaster.Root position="bottom-right">
      <PrimeToaster.Portal>
        <PrimeToaster.Region unstyled>
          <ToastItems />
        </PrimeToaster.Region>
      </PrimeToaster.Portal>
    </PrimeToaster.Root>
  );
}

export const toast = {
  success(message: string) {
    primeToast.success({ title: message, duration: 3500 });
  },
  error(message: string) {
    primeToast.error({ title: message, duration: 5000 });
  },
};

export { Toaster };
