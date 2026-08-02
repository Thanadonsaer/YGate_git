"use client";

import * as React from "react";
import { Toast } from "primereact/toast";

let activeToast: Toast | null = null;

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
        message: {
          className:
            "rounded-[var(--radius-md)] border border-line bg-surface px-4 py-3 text-sm text-ink shadow-[var(--shadow-lg)]",
        },
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
