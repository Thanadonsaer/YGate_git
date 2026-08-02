"use client";

import * as React from "react";
import { TabView, TabPanel } from "primereact/tabview";
import { collectByType } from "./collect-children";

type TriggerProps = { value: string; children?: React.ReactNode };
type ContentProps = { value: string; children?: React.ReactNode };

function TabsList({ children }: { "aria-label"?: string; className?: string; children?: React.ReactNode }) {
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
  const triggers = collectByType<TriggerProps>(children, TabsTrigger);
  const contents = collectByType<ContentProps>(children, TabsContent);
  const values = triggers.map((trigger) => trigger.props.value);
  const [uncontrolled, setUncontrolled] = React.useState(defaultValue ?? values[0]);
  const active = value ?? uncontrolled;
  const activeIndex = Math.max(0, values.indexOf(active));

  function handleTabChange(index: number) {
    const next = values[index];
    if (value === undefined) setUncontrolled(next);
    onValueChange?.(next);
  }

  return (
    <TabView
      activeIndex={activeIndex}
      onTabChange={(event) => handleTabChange(event.index)}
      unstyled
      pt={{
        nav: { className: "inline-flex h-10 items-center gap-0.5 rounded-[var(--radius-sm)] border border-line bg-canvas p-1" },
        inkbar: { className: "hidden" },
      }}
    >
      {triggers.map((trigger) => (
        <TabPanel
          key={trigger.props.value}
          header={trigger.props.children}
          headerClassName="inline-flex h-[30px] items-center gap-1.5 rounded-[calc(var(--radius-sm)-2px)] px-3 text-xs font-bold text-ink-soft"
          pt={{
            headerAction: { className: "tab-trigger flex h-full items-center gap-1.5 px-1" },
          }}
        >
          {contents.find((content) => content.props.value === trigger.props.value)?.props.children}
        </TabPanel>
      ))}
    </TabView>
  );
}

export { Tabs, TabsList, TabsTrigger, TabsContent };
