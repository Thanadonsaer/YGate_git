import type { AccentColor } from "./types";

export const ACCENT_PRESETS: Record<AccentColor, { label: string; action: string; focus: string }> = {
  teal: { label: "Teal (ค่าเริ่มต้น)", action: "#0e5c73", focus: "#1c8fb0" },
  indigo: { label: "Indigo", action: "#3730a3", focus: "#4f46e5" },
  emerald: { label: "Emerald", action: "#0d6b4f", focus: "#17936b" },
  amber: { label: "Amber", action: "#92590b", focus: "#b9770e" },
  rose: { label: "Rose", action: "#9f2b56", focus: "#c2447a" },
};

export function applyAccentColor(accentColor: AccentColor) {
  const preset = ACCENT_PRESETS[accentColor] ?? ACCENT_PRESETS.teal;
  document.documentElement.style.setProperty("--action", preset.action);
  document.documentElement.style.setProperty("--focus", preset.focus);
}
