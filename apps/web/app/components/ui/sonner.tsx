// apps/web/app/components/ui/sonner.tsx
"use client";

import { Toaster as Sonner, type ToasterProps } from "sonner";

function Toaster(props: ToasterProps) {
  return (
    <Sonner
      theme="light"
      position="bottom-right"
      toastOptions={{
        classNames: {
          toast: "rounded-[var(--radius-md)] border border-line bg-surface text-ink shadow-[var(--shadow-lg)] font-body",
          title: "font-bold",
          description: "text-ink-soft",
          success: "border-success/30",
          error: "border-danger/30",
        },
      }}
      {...props}
    />
  );
}

export { Toaster };
export { toast } from "sonner";
