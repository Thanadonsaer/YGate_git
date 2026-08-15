"use client";

/**
 * Figma-style chrome around the SCADA canvas: the tool strip, the zoom widget,
 * the layers panel and the two drag overlays (alignment guides, rubber band).
 *
 * Split out of scada-page.tsx purely for size -- the node components, bindings
 * and persistence all stay there. Everything here is presentational: state lives
 * in ScadaCanvas, which owns the design document.
 */

import {
  ChevronRight,
  Circle,
  Eye,
  EyeOff,
  Hand,
  Lock,
  Minus,
  MousePointer2,
  Plus,
  Spline,
  Square,
  SquareDashed,
  Type as TypeIcon,
  Unlock,
} from "lucide-react";
import { useState, type DragEvent } from "react";
import { Button } from "../../components/ui/button";
import type { Guide } from "../../lib/canvas-geometry";
import type { ScadaNodeData, ScadaNodeType } from "../../lib/types";

/**
 * Active pointer mode. "draw" carries the node it will create so the Insert menu
 * and the dedicated shape/text/section buttons share one code path.
 */
export type CanvasTool =
  | { kind: "move" }
  | { kind: "hand" }
  | { kind: "connector" }
  | { kind: "draw"; node: ScadaNodeType; label: string; shapeKind?: ScadaNodeData["shapeKind"] };

export const MOVE_TOOL: CanvasTool = { kind: "move" };

/** Tools with their own button and single-key shortcut, in strip order. */
const primaryTools: Array<{ tool: CanvasTool; key: string; icon: typeof Square; title: string }> = [
  { tool: { kind: "move" }, key: "v", icon: MousePointer2, title: "Move (V)" },
  { tool: { kind: "hand" }, key: "h", icon: Hand, title: "Hand — pan (H หรือกด Space ค้าง)" },
  { tool: { kind: "draw", node: "section", label: "Section" }, key: "f", icon: SquareDashed, title: "Section (F)" },
  { tool: { kind: "draw", node: "shape", label: "Rectangle", shapeKind: "rectangle" }, key: "r", icon: Square, title: "Rectangle (R)" },
  { tool: { kind: "draw", node: "shape", label: "Ellipse", shapeKind: "circle" }, key: "o", icon: Circle, title: "Ellipse (O)" },
  { tool: { kind: "draw", node: "label", label: "Text" }, key: "t", icon: TypeIcon, title: "Text (T)" },
  { tool: { kind: "connector" }, key: "x", icon: Spline, title: "Connector (X) — คลิกต้นทางแล้วคลิกปลายทาง" },
];

/** Single-key tool shortcut lookup, so the canvas keydown handler stays a one-liner. */
export function toolForKey(key: string): CanvasTool | undefined {
  return primaryTools.find((entry) => entry.key === key)?.tool;
}

export function sameTool(a: CanvasTool, b: CanvasTool) {
  return a.kind === b.kind && (a.kind !== "draw" || b.kind !== "draw" || (a.node === b.node && a.shapeKind === b.shapeKind));
}

/** CSS cursor for the stage. Drawing tools use a crosshair like Figma's. */
export function toolCursor(tool: CanvasTool, spaceHeld: boolean) {
  if (spaceHeld || tool.kind === "hand") return "grab";
  if (tool.kind === "draw") return "crosshair";
  if (tool.kind === "connector") return "cell";
  return "default";
}

export function CanvasToolbar({ tool, sticky, insertEntries, onTool, onSticky }: {
  tool: CanvasTool;
  /** True when the tool is pinned (Alt-picked), so it survives one use. */
  sticky: boolean;
  insertEntries: Array<{ type: ScadaNodeType; title: string; description: string; icon: typeof Square; disabled?: boolean; disabledReason?: string }>;
  onTool: (tool: CanvasTool, sticky: boolean) => void;
  onSticky: (sticky: boolean) => void;
}) {
  const [insertOpen, setInsertOpen] = useState(false);
  return <div className="scada-toolbar" role="toolbar" aria-label="เครื่องมือ canvas">
    {primaryTools.map(({ tool: entry, icon: Icon, title }) => <Button
      key={title}
      variant="icon"
      className={sameTool(tool, entry) ? "tool-active" : undefined}
      aria-pressed={sameTool(tool, entry)}
      title={`${title}${entry.kind === "draw" ? " — Alt+คลิกเพื่อปักหมุดเครื่องมือ" : ""}`}
      aria-label={title}
      onClick={(event) => onTool(entry, event.altKey)}
    ><Icon size={16} /></Button>)}

    <span className="scada-toolbar-divider" />

    <div className="scada-insert">
      <Button variant="icon" compact aria-expanded={insertOpen} aria-haspopup="menu" title="แทรก Node (Insert)" aria-label="แทรก Node" onClick={() => setInsertOpen((open) => !open)}>
        <Plus size={16} /><ChevronRight size={13} className={insertOpen ? "rotate-90 transition-transform" : "transition-transform"} />
      </Button>
      {insertOpen && <div className="scada-insert-menu" role="menu" onMouseLeave={() => setInsertOpen(false)}>
        {insertEntries.map(({ type, title, description, icon: Icon, disabled, disabledReason }) => <button
          key={type}
          role="menuitem"
          disabled={disabled}
          title={disabled ? disabledReason : `${title} — ลากบน canvas เพื่อกำหนดขนาด`}
          onClick={(event) => { onTool({ kind: "draw", node: type, label: title }, event.altKey); setInsertOpen(false); }}
        ><Icon size={16} /><span><strong>{title}</strong><small>{description}</small></span></button>)}
      </div>}
    </div>

    {tool.kind === "draw" && <span className="scada-toolbar-hint">
      ลากเพื่อวาด <strong>{tool.label}</strong>
      <button type="button" onClick={() => onSticky(!sticky)} title="ปักหมุดเครื่องมือไว้วาดต่อเนื่อง">{sticky ? "ปักหมุดอยู่" : "ใช้ครั้งเดียว"}</button>
    </span>}
  </div>;
}

export function ZoomControl({ zoom, onZoom, onFit, onZoomSelection, onReset, hasSelection }: {
  zoom: number;
  onZoom: (direction: 1 | -1) => void;
  onFit: () => void;
  onZoomSelection: () => void;
  onReset: () => void;
  hasSelection: boolean;
}) {
  const [open, setOpen] = useState(false);
  return <div className="scada-zoom">
    <Button variant="icon" compact onClick={() => onZoom(-1)} title="ซูมออก (Ctrl+-)" aria-label="ซูมออก"><Minus size={14} /></Button>
    <button type="button" className="scada-zoom-value" aria-expanded={open} aria-haspopup="menu" onClick={() => setOpen((value) => !value)}>{Math.round(zoom * 100)}%</button>
    <Button variant="icon" compact onClick={() => onZoom(1)} title="ซูมเข้า (Ctrl+=)" aria-label="ซูมเข้า"><Plus size={14} /></Button>
    {open && <div className="scada-zoom-menu" role="menu" onMouseLeave={() => setOpen(false)}>
      <button role="menuitem" onClick={() => { onFit(); setOpen(false); }}>Zoom to fit<kbd>Shift 1</kbd></button>
      <button role="menuitem" disabled={!hasSelection} onClick={() => { onZoomSelection(); setOpen(false); }}>Zoom to selection<kbd>Shift 2</kbd></button>
      <button role="menuitem" onClick={() => { onReset(); setOpen(false); }}>Zoom to 100%<kbd>Shift 0</kbd></button>
    </div>}
  </div>;
}

/** Where a layers-panel drop lands relative to the row it was released on. */
export type LayerDrop = "above" | "below" | "inside";

export type LayerRow = {
  id: string;
  type: ScadaNodeType;
  label: string;
  depth: number;
  hidden: boolean;
  locked: boolean;
  /** True for section/group rows -- only these accept an "inside" drop. */
  container: boolean;
};

/**
 * Figma-style object tree. Rows arrive already flattened and ordered topmost
 * first by the caller, which owns the z-index and parent relationships.
 */
export function LayersPanel({ rows, selectedIDs, icons, onSelect, onToggleHidden, onToggleLocked, onRename, onReorder }: {
  rows: LayerRow[];
  selectedIDs: string[];
  icons: Partial<Record<ScadaNodeType, typeof Square>>;
  onSelect: (id: string, additive: boolean) => void;
  onToggleHidden: (id: string) => void;
  onToggleLocked: (id: string) => void;
  onRename: (id: string, label: string) => void;
  onReorder: (dragID: string, targetID: string, drop: LayerDrop) => void;
}) {
  const [dragID, setDragID] = useState("");
  const [over, setOver] = useState<{ id: string; drop: LayerDrop } | null>(null);
  const [editing, setEditing] = useState("");

  /** Top third inserts above, bottom third below, middle drops inside a container. */
  function dropZone(event: DragEvent<HTMLDivElement>, row: LayerRow): LayerDrop {
    const bounds = event.currentTarget.getBoundingClientRect();
    const ratio = (event.clientY - bounds.top) / bounds.height;
    if (row.container && ratio > 0.3 && ratio < 0.7) return "inside";
    return ratio < 0.5 ? "above" : "below";
  }

  return <aside className="scada-layers" aria-label="Layers">
    <header><SquareDashed size={17} /><div><strong>Layers</strong><small>ลากเพื่อสลับลำดับหรือย้ายเข้ากลุ่ม</small></div></header>
    <div className="scada-layer-list" onDragLeave={() => setOver(null)}>
      {rows.map((row) => {
        const Icon = icons[row.type] || Square;
        const active = selectedIDs.includes(row.id);
        return <div
          key={row.id}
          className={`scada-layer-row${active ? " selected" : ""}${dragID === row.id ? " dragging" : ""}${over?.id === row.id ? ` drop-${over.drop}` : ""}`}
          style={{ paddingLeft: 8 + row.depth * 14 }}
          draggable={editing !== row.id}
          onDragStart={(event) => { setDragID(row.id); event.dataTransfer.effectAllowed = "move"; }}
          onDragEnd={() => { setDragID(""); setOver(null); }}
          onDragOver={(event) => { if (!dragID || dragID === row.id) return; event.preventDefault(); event.dataTransfer.dropEffect = "move"; setOver({ id: row.id, drop: dropZone(event, row) }); }}
          onDrop={(event) => { event.preventDefault(); if (dragID && dragID !== row.id) onReorder(dragID, row.id, dropZone(event, row)); setDragID(""); setOver(null); }}
          onClick={(event) => onSelect(row.id, event.shiftKey || event.ctrlKey || event.metaKey)}
          onDoubleClick={() => setEditing(row.id)}
        >
          <Icon size={14} />
          {editing === row.id
            ? <input
                autoFocus
                defaultValue={row.label}
                maxLength={100}
                onClick={(event) => event.stopPropagation()}
                onBlur={(event) => { onRename(row.id, event.target.value.trim() || row.label); setEditing(""); }}
                onKeyDown={(event) => {
                  if (event.key === "Enter") event.currentTarget.blur();
                  else if (event.key === "Escape") setEditing("");
                }}
              />
            : <span title={row.label}>{row.label}</span>}
          <button type="button" title={row.hidden ? "แสดง" : "ซ่อน"} aria-label={row.hidden ? "แสดง Layer" : "ซ่อน Layer"} onClick={(event) => { event.stopPropagation(); onToggleHidden(row.id); }}>{row.hidden ? <EyeOff size={13} /> : <Eye size={13} />}</button>
          <button type="button" title={row.locked ? "ปลดล็อก" : "ล็อก"} aria-label={row.locked ? "ปลดล็อก Layer" : "ล็อก Layer"} onClick={(event) => { event.stopPropagation(); onToggleLocked(row.id); }}>{row.locked ? <Lock size={13} /> : <Unlock size={13} />}</button>
        </div>;
      })}
      {rows.length === 0 && <p className="scada-layer-empty">ยังไม่มี object — เลือกเครื่องมือแล้วลากวาดบน canvas</p>}
    </div>
  </aside>;
}

/**
 * Alignment guides, drawn in flow coordinates. Rendered inside React Flow's
 * ViewportPortal by the caller so they pan and zoom with the canvas; widths are
 * divided by zoom to stay hairline-thin at any scale.
 */
export function GuideOverlay({ guides, zoom }: { guides: Guide[]; zoom: number }) {
  if (guides.length === 0) return null;
  const thickness = 1 / zoom;
  return <>{guides.map((guide, index) => {
    const style = guide.axis === "x"
      ? { left: guide.at, top: guide.start, width: thickness, height: guide.end - guide.start }
      : { left: guide.start, top: guide.at, width: guide.end - guide.start, height: thickness };
    return <div key={index} className="scada-guide" style={{ position: "absolute", ...style }}>
      {guide.gap > 0 && <span className="scada-guide-gap" style={{ transform: `scale(${1 / zoom})` }}>{guide.gap}</span>}
    </div>;
  })}</>;
}

/** Rubber band shown while a drawing tool is dragging out a new node, in screen pixels. */
export function DrawPreview({ rect, label }: { rect: { left: number; top: number; width: number; height: number } | null; label: string }) {
  if (!rect) return null;
  return <div className="scada-draw-preview" style={rect}>
    <span>{label} · {Math.round(rect.width)} × {Math.round(rect.height)}</span>
  </div>;
}
