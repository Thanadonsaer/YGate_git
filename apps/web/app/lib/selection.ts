/**
 * Content equality for a node-selection list.
 *
 * React Flow calls `onSelectionChange` on every internal store sync, not only
 * when the user clicks. Feeding it `nodes.map(n => n.id)` hands React a freshly
 * allocated array every time, so `Object.is` never matches, React can never bail
 * out of the update, and each render re-syncs the store and fires the callback
 * again -- an unbounded update loop (React error #185) that took down both
 * /scada and /scada/live. Callers use this to keep the previous array when the
 * selection is unchanged, which restores the bail-out.
 */
export function sameSelection(a: readonly string[], b: readonly string[]) {
  return a.length === b.length && a.every((id, index) => id === b[index]);
}
