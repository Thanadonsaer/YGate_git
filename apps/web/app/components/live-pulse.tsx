"use client";

import type { ConnectionState } from "../lib/types";
import { cn } from "../lib/cn";

const PATHS: Record<ConnectionState, string> = {
  connected: "M0 12 L10 12 L15 3 L20 21 L25 6 L30 18 L35 12 L44 12 L49 3 L54 21 L59 6 L64 18 L69 12 L78 12 L83 3 L88 21 L93 6 L98 18 L103 12 L112 12",
  connecting: "M0 12 L10 12 L14 8 L18 17 L22 5 L26 19 L30 12 L34 15 L38 9 L44 12 L52 12 L56 6 L60 18 L64 12 L70 14 L76 12 L82 12 L86 4 L90 20 L94 12 L100 12 L112 12",
  offline: "M0 12 L112 12",
};

export function LivePulse({ state, className }: { state: ConnectionState; className?: string }) {
  return (
    <svg
      className={cn("live-pulse", `live-pulse-${state}`, className)}
      viewBox="0 0 112 24"
      width="56"
      height="12"
      role="img"
      aria-label={state === "connected" ? "เชื่อมต่อ live" : state === "connecting" ? "กำลังเชื่อมต่อ" : "ออฟไลน์"}
    >
      <path d={PATHS[state]} fill="none" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
