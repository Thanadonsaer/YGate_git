/**
 * Pure canvas math for the SCADA builder's Figma-style interactions: alignment
 * guides while dragging, layer z-order resequencing, and parent rebasing when a
 * node is dragged in or out of a group.
 *
 * Deliberately free of React Flow types so it runs under `node --test` with no
 * DOM -- the caller adapts its nodes into the plain Rect/Point shapes here.
 */

export type Point = { x: number; y: number };
export type Rect = Point & { width: number; height: number };

/** One alignment line drawn while dragging, plus the gap it measures. */
export type Guide = {
  /** "x" is a vertical line at a fixed x; "y" is a horizontal line at a fixed y. */
  axis: "x" | "y";
  /** Flow coordinate the line sits at. */
  at: number;
  /** Extent of the line along the other axis, so it spans both rects. */
  start: number;
  end: number;
  /** Edge-to-edge distance between the two rects along the other axis, px. 0 when they overlap. */
  gap: number;
};

export const SNAP_THRESHOLD = 6;

/** Leading edge, centre and trailing edge of a rect along one axis. */
function edges(rect: Rect, axis: "x" | "y") {
  const start = axis === "x" ? rect.x : rect.y;
  const size = axis === "x" ? rect.width : rect.height;
  return [start, start + size / 2, start + size];
}

/**
 * Snap `moving` to the edges and centres of `others`, Figma-style: per axis the
 * closest candidate within `threshold` wins, and every snapped axis produces one
 * guide line spanning both rects.
 *
 * Returns the corrected top-left position -- callers keep the raw pointer
 * position when `guides` is empty, so a drag with nothing nearby is unaffected.
 */
export function snapToGuides(moving: Rect, others: Rect[], threshold = SNAP_THRESHOLD): { position: Point; guides: Guide[] } {
  const position: Point = { x: moving.x, y: moving.y };
  const guides: Guide[] = [];

  for (const axis of ["x", "y"] as const) {
    let best: { delta: number; at: number; other: Rect } | null = null;
    for (const other of others) {
      for (const target of edges(other, axis)) {
        for (const edge of edges(moving, axis)) {
          const delta = target - edge;
          if (Math.abs(delta) > threshold) continue;
          if (!best || Math.abs(delta) < Math.abs(best.delta)) best = { delta, at: target, other };
        }
      }
    }
    if (!best) continue;
    position[axis] = moving[axis] + best.delta;

    // The line runs along the *other* axis, spanning from the topmost/leftmost
    // edge of the pair to the bottommost/rightmost, exactly like Figma's.
    const snapped: Rect = axis === "x" ? { ...moving, x: position.x } : { ...moving, y: position.y };
    const mineStart = axis === "x" ? snapped.y : snapped.x;
    const mineEnd = mineStart + (axis === "x" ? snapped.height : snapped.width);
    const theirsStart = axis === "x" ? best.other.y : best.other.x;
    const theirsEnd = theirsStart + (axis === "x" ? best.other.height : best.other.width);
    guides.push({
      axis,
      at: best.at,
      start: Math.min(mineStart, theirsStart),
      end: Math.max(mineEnd, theirsEnd),
      gap: Math.max(0, Math.round(Math.max(mineStart, theirsStart) - Math.min(mineEnd, theirsEnd))),
    });
  }

  return { position, guides };
}

/**
 * Move `id` so it sits directly below `beforeId` in a bottom-to-top sibling
 * list, or to the very top when `beforeId` is null. Returns the list unchanged
 * when either id is missing, so a stray drop is a no-op rather than a reshuffle.
 */
export function reorder(ids: string[], id: string, beforeId: string | null): string[] {
  if (!ids.includes(id)) return ids;
  const without = ids.filter((item) => item !== id);
  const index = beforeId === null ? without.length : without.indexOf(beforeId);
  if (index < 0) return ids;
  return [...without.slice(0, index), id, ...without.slice(index)];
}

/**
 * Dense z-index assignment for a bottom-to-top list. Reassigning the whole
 * sibling run keeps ordering stable across repeated drags -- the old
 * "max + 1 / min - 1" bump grew without bound and eventually collided with the
 * -1 that sections and groups default to.
 */
export function resequenceZ(bottomToTop: string[]): Record<string, number> {
  return Object.fromEntries(bottomToTop.map((id, index) => [id, index]));
}

/** Absolute canvas position given a chain of ancestor origins (nearest parent first). */
export function absolutePosition(position: Point, ancestorOrigins: Point[]): Point {
  return ancestorOrigins.reduce((acc, origin) => ({ x: acc.x + origin.x, y: acc.y + origin.y }), position);
}

/** Re-express an absolute position relative to a new parent's absolute origin. */
export function relativePosition(absolute: Point, parentOrigin: Point): Point {
  return { x: absolute.x - parentOrigin.x, y: absolute.y - parentOrigin.y };
}

/** Axis-locked drag offset for Shift+drag: whichever axis moved further wins. */
export function lockAxis(from: Point, to: Point): Point {
  return Math.abs(to.x - from.x) >= Math.abs(to.y - from.y) ? { x: to.x, y: from.y } : { x: from.x, y: to.y };
}
