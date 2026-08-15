# SCADA Builder Canvas — Figma-style UX

**Date:** 2026-08-15 · **Status:** implemented

## Problem

The SCADA builder canvas ([apps/web/app/features/scada/scada-page.tsx](../../../apps/web/app/features/scada/scada-page.tsx))
was a click-to-add palette: clicking a palette row dropped a node at a cascading
offset, with no way to place or size it where you meant. Navigation, selection
and manipulation all differed from the whiteboard tools people already know.

## Decision

Keep React Flow (`@xyflow/react`) and add a tool-state layer on top of it. The
alternatives — tldraw, or a custom konva canvas — buy the interaction model for
free but throw away 13 node types, telemetry bindings, edges, draft/publish
persistence and undo. Not worth it.

## Scope

**Tools.** A strip in the existing stage toolbar: Move `V`, Hand `H`,
Section `F`, Rectangle `R`, Ellipse `O`, Text `T`, Connector `X`, plus an
**Insert ▾** popover holding all 13 node types. Picking a tool arms
drag-to-draw; a click without a drag falls back to the type's default size.
The tool reverts to Move after one use unless Alt-picked (pinned).

**Navigation.** Wheel pans, Ctrl+wheel zooms, Space or the middle button pans,
left-drag on empty canvas marquee-selects. A zoom widget replaces React Flow's
`<Controls>`: Shift+1 fit, Shift+2 zoom to selection, Shift+0 100%, Ctrl+`=`/`-`.

**Manipulation.** Alt+drag stamps a copy and drags the originals; Shift+drag
locks an axis; arrows nudge 1px and Shift+arrows 10px; alignment guides snap
within 6px to neighbours' edges and centres and show the gap in px; Ctrl
suppresses snapping; Ctrl+G / Ctrl+Shift+G group and ungroup. The old 20px grid
survives as the fallback when no guide is in range.

**Layers panel.** Replaces the palette. Object tree ordered topmost-first,
nested under containers, with rename (double-click), hide and lock toggles, and
drag to reorder (`zIndex`) or reparent into a group/section (`parentId`, with
position rebasing). Every drag pushes an undo checkpoint.

## Files

| File | Role |
|---|---|
| `apps/web/app/lib/canvas-geometry.ts` | Pure snap/reorder/rebase math, no React Flow types |
| `apps/web/app/lib/canvas-geometry.test.ts` | 10 tests under `node --test` |
| `apps/web/app/features/scada/scada-canvas-chrome.tsx` | Toolbar, zoom widget, layers panel, guide + rubber-band overlays |
| `apps/web/app/features/scada/scada-page.tsx` | `ScadaCanvas` wiring; node components and inspector untouched |
| `apps/web/app/globals.css` | `.scada-layers`, `.scada-toolbar`, `.scada-zoom`, `.scada-guide`, `.scada-draw-preview` |

`ScadaDesign` is unchanged — reordering writes the existing `zIndex` and
`parentId` fields, so saved drafts load as before.

## Deliberately excluded

Pen/vector tool, comments, sticky notes, components/variants, auto-layout,
multiplayer cursors, multi-row layer drag, auto-scroll while dragging in the
panel, Alt+hover distance measurement (distances show during a drag instead).

## Known ceiling

Alignment guides re-walk every node's ancestor chain per drag frame — O(n²),
comfortably under a frame at a few hundred nodes. Marked with a `ponytail:`
comment at the call site; memoise absolute rects on drag start if screens grow.
