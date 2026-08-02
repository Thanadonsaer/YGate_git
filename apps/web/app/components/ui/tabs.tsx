"use client";

import * as React from "react";
import { Tabs as PrimeTabs } from "primereact/tabs";
import type { TabsRootChangeEvent } from "@primereact/types/primitive/tabs";
import { collectByType } from "./collect-children";

type TriggerProps = { value: string; children?: React.ReactNode };
type ContentProps = { value: string; children?: React.ReactNode };
type ListProps = { "aria-label"?: string; className?: string; children?: React.ReactNode };

function TabsList({ children }: ListProps) {
  return <>{children}</>;
}

function TabsTrigger({ children }: TriggerProps) {
  return <>{children}</>;
}

function TabsContent({ children }: ContentProps) {
  return <>{children}</>;
}

function Tabs({
  value,
  defaultValue,
  onValueChange,
  children,
}: {
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  children: React.ReactNode;
}) {
  const [list] = collectByType<ListProps>(children, TabsList);
  const triggers = collectByType<TriggerProps>(children, TabsTrigger);
  const contents = collectByType<ContentProps>(children, TabsContent);

  return (
    <PrimeTabs.Root
      className="flex flex-col gap-2"
      value={value}
      defaultValue={defaultValue}
      onValueChange={(event: TabsRootChangeEvent) => {
        if (event.value !== undefined) onValueChange?.(String(event.value));
      }}
    >
      <PrimeTabs.List
        className="inline-flex h-10 items-center gap-0.5 rounded-[var(--radius-sm)] border border-line bg-canvas p-1"
        pt={{ root: { "aria-label": list?.props["aria-label"] } }}
      >
        {triggers.map((trigger) => (
          <PrimeTabs.Tab
            key={trigger.props.value}
            value={trigger.props.value}
            className="inline-flex h-[30px] items-center gap-1.5 rounded-[calc(var(--radius-sm)-2px)] px-3 text-xs font-bold text-ink-soft transition data-[active]:bg-surface data-[active]:text-nav data-[active]:shadow-[var(--shadow-sm)]"
          >
            {trigger.props.children}
          </PrimeTabs.Tab>
        ))}
      </PrimeTabs.List>
      {contents.length > 0 && (
        <PrimeTabs.Panels>
          {contents.map((content) => (
            <PrimeTabs.Panel key={content.props.value} value={content.props.value}>
              {content.props.children}
            </PrimeTabs.Panel>
          ))}
        </PrimeTabs.Panels>
      )}
    </PrimeTabs.Root>
  );
}

export { Tabs, TabsList, TabsTrigger, TabsContent };
