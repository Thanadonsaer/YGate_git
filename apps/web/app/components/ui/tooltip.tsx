"use client";

import * as React from "react";
import { Tooltip as PrimeTooltip } from "primereact/tooltip";
import { collectByType } from "./collect-children";
import { cn } from "../../lib/cn";

// PrimeReact positions the bubble in a portal, so it can no longer be clipped
// by a toolbar with `overflow: hidden` the way the old absolutely-positioned
// span could. The trigger is matched by a per-instance class name because
// PrimeReact's Tooltip binds to a CSS selector rather than a React ref.

function TooltipTrigger({ children }: { asChild?: boolean; children: React.ReactElement }) {
  return children;
}

function TooltipContent({ children }: { className?: string; children: React.ReactNode }) {
  return <>{children}</>;
}

function Tooltip({ children }: { children: React.ReactNode }) {
  const target = "tt" + React.useId().replace(/[^a-zA-Z0-9]/g, "");
  const triggers = collectByType<{ children: React.ReactElement }>(children, TooltipTrigger);
  const contents = collectByType<{ children: React.ReactNode }>(children, TooltipContent);
  const trigger = triggers[0]?.props.children;
  if (!trigger) return null;

  return (
    <>
      {React.cloneElement(trigger, {
        className: cn((trigger.props as { className?: string }).className, target),
      } as React.HTMLAttributes<HTMLElement>)}
      <PrimeTooltip
        target={"." + target}
        position="bottom"
        showDelay={120}
        pt={{
          root: { className: "absolute z-50 max-w-72" },
          text: { className: "block rounded-[var(--radius-sm)] bg-ink px-2.5 py-1.5 text-xs font-semibold leading-snug text-surface shadow-[var(--shadow-sm)]" },
          arrow: { className: "hidden" },
        }}
      >
        {contents[0]?.props.children}
      </PrimeTooltip>
    </>
  );
}

export { Tooltip, TooltipTrigger, TooltipContent };
