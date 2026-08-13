"use client";

import * as React from "react";
import { downsample, nearestIndex, timeTicks, valueTicks, type Point } from "../../lib/telemetry-math";

export const SERIES_COLORS = ["#0e5c73", "#e8a33d", "#16825c", "#a84dd6", "#c0392b", "#1c8fb0"];

export type ChartSeries = {
  key: string;
  label: string;
  unit: string;
  decimals: number;
  color: string;
  points: Point[];
};

const MARGIN = { top: 10, right: 14, bottom: 32, left: 60 };

const clockFormat = new Intl.DateTimeFormat("th-TH", { hour: "2-digit", minute: "2-digit", hour12: false });
const dayFormat = new Intl.DateTimeFormat("th-TH", { day: "numeric", month: "short" });
const stampFormat = new Intl.DateTimeFormat("th-TH", { dateStyle: "medium", timeStyle: "short" });

/**
 * Series sharing an axis must share a unit -- kW and Hz on one axis makes both
 * unreadable. Rather than juggling dual axes, each unit gets its own stacked
 * panel over a shared time axis and a shared crosshair.
 */
export function TimeSeriesChart({
  series,
  from,
  to,
  onZoom,
  onResetZoom,
  loading = false,
  height = 240,
}: {
  series: ChartSeries[];
  from: number;
  to: number;
  onZoom?: (from: Date, to: Date) => void;
  onResetZoom?: () => void;
  loading?: boolean;
  height?: number;
}) {
  const [hoverAt, setHoverAt] = React.useState<number | null>(null);

  const groups: Array<{ unit: string; series: ChartSeries[] }> = [];
  for (const entry of series) {
    const group = groups.find((candidate) => candidate.unit === entry.unit);
    if (group) group.series.push(entry);
    else groups.push({ unit: entry.unit, series: [entry] });
  }

  if (groups.length === 0) return null;

  return (
    <div className="ts-stack">
      {groups.map((group) => (
        <ChartPanel
          key={group.unit || "__none"}
          unit={group.unit}
          series={group.series}
          from={from}
          to={to}
          height={height}
          hoverAt={hoverAt}
          onHoverAt={setHoverAt}
          onZoom={onZoom}
          onResetZoom={onResetZoom}
          loading={loading}
        />
      ))}
      <p className="ts-axis-caption">
        {stampFormat.format(new Date(from))} – {stampFormat.format(new Date(to))}
        {onZoom && <span className="ts-hint"> · ลากบนกราฟเพื่อ zoom · ดับเบิลคลิกเพื่อรีเซ็ต</span>}
      </p>
    </div>
  );
}

function ChartPanel({
  unit,
  series,
  from,
  to,
  height,
  hoverAt,
  onHoverAt,
  onZoom,
  onResetZoom,
  loading,
}: {
  unit: string;
  series: ChartSeries[];
  from: number;
  to: number;
  height: number;
  hoverAt: number | null;
  onHoverAt: (at: number | null) => void;
  onZoom?: (from: Date, to: Date) => void;
  onResetZoom?: () => void;
  loading: boolean;
}) {
  const containerRef = React.useRef<HTMLDivElement>(null);
  const [width, setWidth] = React.useState(0);
  const [drag, setDrag] = React.useState<{ start: number; current: number } | null>(null);
  const [active, setActive] = React.useState(false);

  React.useEffect(() => {
    const element = containerRef.current;
    if (!element) return;
    const observer = new ResizeObserver((entries) => setWidth(entries[0].contentRect.width));
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  const plotted = React.useMemo(
    () => series.map((entry) => ({ ...entry, points: downsample(entry.points) })),
    [series],
  );

  const innerWidth = Math.max(width - MARGIN.left - MARGIN.right, 10);
  const innerHeight = Math.max(height - MARGIN.top - MARGIN.bottom, 10);
  const [low, high] = valueDomain(plotted);
  const span = Math.max(to - from, 1);

  const scaleX = (t: number) => MARGIN.left + ((t - from) / span) * innerWidth;
  const scaleY = (v: number) => MARGIN.top + innerHeight - ((v - low) / (high - low)) * innerHeight;
  const timeAt = (px: number) => {
    const ratio = (px - MARGIN.left) / innerWidth;
    return from + Math.min(Math.max(ratio, 0), 1) * span;
  };
  const pixelAt = (event: React.PointerEvent<SVGSVGElement>) =>
    event.clientX - event.currentTarget.getBoundingClientRect().left;

  const hasData = plotted.some((entry) => entry.points.length > 0);

  return (
    <div className="ts-panel" ref={containerRef}>
      <div className="ts-panel-head">
        <span className="ts-unit">{unit || "ไม่มีหน่วย"}</span>
        <div className="ts-legend">
          {plotted.map((entry) => {
            const latest = entry.points[entry.points.length - 1];
            return (
              <span key={entry.key} className="ts-legend-item" title={entry.label}>
                <i className="ts-swatch" style={{ background: entry.color }} />
                <span className="ts-legend-label">{entry.label}</span>
                <strong>{latest ? latest.v.toFixed(entry.decimals) : "-"}</strong>
              </span>
            );
          })}
        </div>
      </div>

      <div className="ts-plot">
        {width === 0 || !hasData ? (
          <div className="ts-empty">{width === 0 ? "" : loading ? "กำลังโหลดข้อมูล…" : "ไม่มีข้อมูลในช่วงเวลานี้"}</div>
        ) : (
          <svg
          className="ts-svg"
          width={width}
          height={height}
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label={`กราฟ ${plotted.map((entry) => entry.label).join(", ")}`}
          onPointerDown={(event) => {
            if (event.button !== 0 || !onZoom) return;
            event.currentTarget.setPointerCapture(event.pointerId);
            const px = pixelAt(event);
            setDrag({ start: px, current: px });
          }}
          onPointerMove={(event) => {
            const px = pixelAt(event);
            setActive(true);
            onHoverAt(timeAt(px));
            if (drag) setDrag({ start: drag.start, current: px });
          }}
          onPointerUp={() => {
            if (drag && Math.abs(drag.current - drag.start) > 8 && onZoom) {
              const a = timeAt(Math.min(drag.start, drag.current));
              const b = timeAt(Math.max(drag.start, drag.current));
              onZoom(new Date(a), new Date(b));
            }
            setDrag(null);
          }}
          onPointerLeave={() => {
            setDrag(null);
            setActive(false);
            onHoverAt(null);
          }}
          onDoubleClick={() => onResetZoom?.()}
        >
          {valueTicks(low, high).map((tick) => (
            <g key={tick}>
              <line className="ts-grid" x1={MARGIN.left} x2={width - MARGIN.right} y1={scaleY(tick)} y2={scaleY(tick)} />
              <text className="ts-tick" x={MARGIN.left - 8} y={scaleY(tick)} textAnchor="end" dominantBaseline="middle">
                {formatTick(tick)}
              </text>
            </g>
          ))}

          {timeTicks(from, to).map((tick) => {
            const date = new Date(tick);
            const midnight = date.getHours() === 0 && date.getMinutes() === 0;
            return (
              <g key={tick}>
                <line className="ts-grid" x1={scaleX(tick)} x2={scaleX(tick)} y1={MARGIN.top} y2={MARGIN.top + innerHeight} />
                <text
                  className={midnight ? "ts-tick ts-tick-day" : "ts-tick"}
                  x={scaleX(tick)}
                  y={height - 10}
                  textAnchor="middle"
                >
                  {midnight ? dayFormat.format(date) : clockFormat.format(date)}
                </text>
              </g>
            );
          })}

          <line className="ts-axis" x1={MARGIN.left} x2={width - MARGIN.right} y1={MARGIN.top + innerHeight} y2={MARGIN.top + innerHeight} />
          <line className="ts-axis" x1={MARGIN.left} x2={MARGIN.left} y1={MARGIN.top} y2={MARGIN.top + innerHeight} />

          {plotted.map((entry) => (
            <path key={entry.key} className="ts-line" d={buildPath(entry.points, scaleX, scaleY)} style={{ stroke: entry.color }} />
          ))}

          {hoverAt !== null && !drag && (
            <line className="ts-crosshair" x1={scaleX(hoverAt)} x2={scaleX(hoverAt)} y1={MARGIN.top} y2={MARGIN.top + innerHeight} />
          )}
          {hoverAt !== null && !drag && plotted.map((entry) => {
            const index = nearestIndex(entry.points, hoverAt);
            if (index < 0) return null;
            const point = entry.points[index];
            return <circle key={entry.key} className="ts-dot" cx={scaleX(point.t)} cy={scaleY(point.v)} r={3} style={{ fill: entry.color }} />;
          })}

          {drag && Math.abs(drag.current - drag.start) > 2 && (
            <rect
              className="ts-selection"
              x={Math.min(drag.start, drag.current)}
              width={Math.abs(drag.current - drag.start)}
              y={MARGIN.top}
              height={innerHeight}
            />
          )}
          </svg>
        )}
        {loading && hasData && <div className="ts-loading-overlay" role="status" aria-live="polite"><span className="ts-loading-track" aria-hidden="true"><i /></span>กำลังโหลดข้อมูล…</div>}
      </div>

      {active && hoverAt !== null && !drag && hasData && (
        <div
          className="ts-tooltip"
          style={{ left: Math.min(Math.max(scaleX(hoverAt) + 12, 4), Math.max(width - 210, 4)) }}
        >
          <strong>{stampFormat.format(new Date(hoverAt))}</strong>
          {plotted.map((entry) => {
            const index = nearestIndex(entry.points, hoverAt);
            if (index < 0) return null;
            const point = entry.points[index];
            return (
              <span key={entry.key}>
                <i className="ts-swatch" style={{ background: entry.color }} />
                <span className="ts-legend-label">{entry.label}</span>
                <strong>{point.v.toFixed(entry.decimals)}{unit ? ` ${unit}` : ""}</strong>
              </span>
            );
          })}
        </div>
      )}
    </div>
  );
}

function valueDomain(series: ChartSeries[]): [number, number] {
  let low = Number.POSITIVE_INFINITY;
  let high = Number.NEGATIVE_INFINITY;
  for (const entry of series) {
    for (const point of entry.points) {
      if (point.v < low) low = point.v;
      if (point.v > high) high = point.v;
    }
  }
  if (!Number.isFinite(low) || !Number.isFinite(high)) return [0, 1];
  if (low === high) return [low - 1, high + 1];
  const padding = (high - low) * 0.08;
  return [low - padding, high + padding];
}

/**
 * One continuous polyline through every sample, gaps included.
 *
 * This used to break the line wherever a gap ran past 4x the median sample
 * interval, so an offline inverter would not look like it held a steady
 * output. That reading of a gap no longer holds: the Middleware now skips
 * enqueueing a reading whose register values have not moved (with a heartbeat
 * every 30 minutes, see IdleHeartbeat in internal/app/service.go), so a missing
 * sample means "unchanged" and joining across it is the accurate picture. A
 * device that is genuinely gone stops sending heartbeats too, and surfaces as a
 * stale observedAt on the plant/device status rather than as a hole here.
 */
function buildPath(points: Point[], scaleX: (t: number) => number, scaleY: (v: number) => number) {
  return points
    .map((point, index) => `${index === 0 ? "M" : "L"}${scaleX(point.t).toFixed(1)},${scaleY(point.v).toFixed(1)}`)
    .join("");
}

function formatTick(value: number) {
  const magnitude = Math.abs(value);
  if (magnitude >= 1000) return value.toLocaleString(undefined, { maximumFractionDigits: 0 });
  if (magnitude >= 10) return value.toFixed(1).replace(/\.0$/, "");
  return value.toFixed(2).replace(/\.?0+$/, "");
}
