"use client";

import { ChartLine, Download, RefreshCw } from "lucide-react";
import { Checkbox, FormMessage, TextInput } from "../../components/ui/form";
import { useCallback, useEffect, useMemo, useState } from "react";
import { api, downloadBlob, errorMessage, toDatetimeLocal } from "../../lib/api";
import { fetchRange, loadRegisterCatalog, pointMeta, type PointMeta } from "../../lib/telemetry-history";
import {
  bucketEnergy,
  classifyUnit,
  previousPeriod,
  seriesToCSV,
  toSeries,
  totalEnergyKWh,
  type Bucket,
  type Point,
} from "../../lib/telemetry-math";
import { SERIES_COLORS, TimeSeriesChart, type ChartSeries } from "../../components/charts/time-series-chart";
import { MultiSelect } from "../../components/ui/multi-select";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { Button } from "../../components/ui/button";
import type { Device, Plant } from "../../lib/types";
import { buildScatterPoints, findSignalKey, type ScatterPoint } from "../../lib/xy-scatter";

const PRESETS = [
  { label: "24 ชม.", hours: 24 },
  { label: "7 วัน", hours: 24 * 7 },
  { label: "30 วัน", hours: 24 * 30 },
];

/** Past this span an hourly bar chart is unreadable, so bars roll up to days. */
const DAILY_BARS_AFTER_MS = 2 * 24 * 60 * 60 * 1000;

function rangeOfHours(hours: number) {
  const to = new Date();
  return { from: new Date(to.getTime() - hours * 3_600_000), to };
}

type Loaded = { seriesByKey: Record<string, Point[]>; truncated: boolean };

export function EnergyAnalysisPage() {
  const [plants, setPlants] = useState<Plant[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [plantId, setPlantId] = useState("");
  const [deviceId, setDeviceId] = useState("");
  const [range, setRange] = useState(() => rangeOfHours(24));
  const [compare, setCompare] = useState(false);
  const [analysisView, setAnalysisView] = useState<"trend" | "xy" | "solar">("trend");
  const [catalog, setCatalog] = useState<Record<string, PointMeta>>({});
  const [current, setCurrent] = useState<Loaded>({ seriesByKey: {}, truncated: false });
  const [baseline, setBaseline] = useState<Record<string, Point[]>>({});
  const [selectedKeys, setSelectedKeys] = useState<string[]>([]);
  const [reloadToken, setReloadToken] = useState(0);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const plant = plants.find((entry) => entry.id === plantId);
  const device = devices.find((entry) => entry.id === deviceId);

  useEffect(() => {
    void api("/api/v1/plants")
      .then(async (response) => {
        if (!response.ok) throw new Error("ไม่สามารถโหลด Plant ได้");
        setPlants((await response.json()) as Plant[]);
      })
      .catch((cause: unknown) => setError(errorMessage(cause)));
  }, []);

  useEffect(() => {
    setDeviceId("");
    setDevices([]);
    if (!plantId) return;
    const controller = new AbortController();
    void api(`/api/v1/plants/${encodeURIComponent(plantId)}/devices`, { signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) throw new Error("ไม่สามารถโหลด Device ได้");
        setDevices((await response.json()) as Device[]);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [plantId]);

  useEffect(() => {
    setCatalog({});
    if (!plantId || !device) return;
    const controller = new AbortController();
    void loadRegisterCatalog(plantId, device, controller.signal)
      .then(setCatalog)
      .catch(() => {
        if (!controller.signal.aborted) setError("ไม่สามารถโหลด Metadata ของ Parameter ได้");
      });
    return () => controller.abort();
  }, [plantId, device]);

  useEffect(() => {
    if (!plantId || !deviceId) return;
    if (range.from >= range.to) {
      setError("ช่วงเวลาไม่ถูกต้อง (start ต้องอยู่ก่อน end)");
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setError("");
    void (async () => {
      try {
        const page = await fetchRange(plantId, deviceId, range.from, range.to, controller.signal);
        setCurrent({ seriesByKey: toSeries(page.readings), truncated: page.truncated });
        if (compare) {
          const window = previousPeriod(range.from, range.to);
          const before = await fetchRange(plantId, deviceId, window.from, window.to, controller.signal);
          setBaseline(toSeries(before.readings));
        } else {
          setBaseline({});
        }
      } catch (cause) {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    })();
    return () => controller.abort();
  }, [plantId, deviceId, range, compare, reloadToken]);

  const availableKeys = useMemo(
    () => Object.keys(current.seriesByKey)
      .filter((key) => catalog[key]?.isEnabled !== false)
      .sort((a, b) => pointMeta(catalog, a).displayName.localeCompare(pointMeta(catalog, b).displayName)),
    [current.seriesByKey, catalog],
  );

  // Default to whatever can actually be analysed -- a power or energy register
  // -- instead of the alphabetically first key, which is usually a voltage.
  useEffect(() => {
    setSelectedKeys((currentKeys) => {
      const kept = currentKeys.filter((key) => availableKeys.includes(key));
      if (kept.length > 0) return kept.length === currentKeys.length ? currentKeys : kept;
      const preferred = availableKeys.filter((key) => classifyUnit(pointMeta(catalog, key).unit) !== "other");
      return (preferred.length > 0 ? preferred : availableKeys).slice(0, 3);
    });
  }, [availableKeys, catalog]);

  const metricKey = useMemo(() => {
    const candidates = selectedKeys.length > 0 ? selectedKeys : availableKeys;
    return candidates.find((key) => classifyUnit(pointMeta(catalog, key).unit) === "power")
      ?? candidates.find((key) => classifyUnit(pointMeta(catalog, key).unit) === "energy");
  }, [selectedKeys, availableKeys, catalog]);

  const metric = metricKey ? pointMeta(catalog, metricKey) : undefined;
  const metricPoints = metricKey ? current.seriesByKey[metricKey] ?? [] : [];
  const basePoints = metricKey ? baseline[metricKey] ?? [] : [];
  const solarXKey = findSignalKey(availableKeys, /irradiance|irradiation|sun/i) ?? selectedKeys[0];
  const solarYKey = findSignalKey(availableKeys, /active.?power|power.?ac|ac.?power/i) ?? selectedKeys[1];
  const analysisXKey = analysisView === "solar" ? solarXKey : selectedKeys[0];
  const analysisYKey = analysisView === "solar" ? solarYKey : selectedKeys[1];
  const analysisPoints = analysisXKey && analysisYKey
    ? buildScatterPoints(current.seriesByKey[analysisXKey] ?? [], current.seriesByKey[analysisYKey] ?? [])
    : [];
  const barStep: "hour" | "day" = range.to.getTime() - range.from.getTime() > DAILY_BARS_AFTER_MS ? "day" : "hour";

  const buckets = useMemo(
    () => (metric ? bucketEnergy(metricPoints, metric.unit, barStep) : []),
    [metricPoints, metric, barStep],
  );
  const baseBuckets = useMemo(
    () => (metric && compare ? bucketEnergy(basePoints, metric.unit, barStep) : []),
    [basePoints, metric, barStep, compare],
  );

  const series = useMemo<ChartSeries[]>(
    () => selectedKeys.map((key, index) => {
      const meta = pointMeta(catalog, key);
      return {
        key,
        label: meta.displayName,
        unit: meta.unit,
        decimals: meta.decimals,
        color: SERIES_COLORS[index % SERIES_COLORS.length],
        points: current.seriesByKey[key] ?? [],
      };
    }),
    [selectedKeys, catalog, current.seriesByKey],
  );

  const exportCSV = useCallback(() => {
    if (selectedKeys.length === 0 || !device) return;
    const columns = selectedKeys.map((key) => {
      const meta = pointMeta(catalog, key);
      return { key, header: meta.unit ? `${meta.displayName} (${meta.unit})` : meta.displayName };
    });
    const csv = seriesToCSV(columns, current.seriesByKey);
    const stamp = toDatetimeLocal(range.from).replace(/[-:]/g, "").replace("T", "-");
    // Excel needs the BOM to read the Thai display names as UTF-8.
    downloadBlob(new Blob([`﻿${csv}`], { type: "text/csv;charset=utf-8" }), `ygate-${device.externalId}-${stamp}.csv`);
  }, [selectedKeys, catalog, current.seriesByKey, device, range.from]);

  return (
    <div className="content energy-analysis-content">
      <div className="section-heading">
        <div><p>Inverter telemetry</p><h2>Energy Analysis</h2></div>
        <div className="heading-actions">
          <Button
            variant="icon"
            onClick={exportCSV}
            disabled={selectedKeys.length === 0 || !device}
            title="ดาวน์โหลด CSV"
            aria-label="ดาวน์โหลด CSV"
          >
            <Download size={18} />
          </Button>
          <Button
            variant="icon"
            onClick={() => setReloadToken((token) => token + 1)}
            disabled={loading || !deviceId}
            title="รีเฟรช"
            aria-label="รีเฟรช"
          >
            <RefreshCw size={18} />
          </Button>
        </div>
      </div>

      <div className="ea-filters">
        <label className="ea-field">Plant
          <Select value={plantId} onValueChange={setPlantId}>
            <SelectTrigger><SelectValue placeholder="เลือก Plant" /></SelectTrigger>
            <SelectContent>{plants.map((entry) => <SelectItem key={entry.id} value={entry.id}>{entry.name} ({entry.code})</SelectItem>)}</SelectContent>
          </Select>
        </label>
        <label className="ea-field">Device
          <Select value={deviceId} onValueChange={setDeviceId} disabled={!plantId}>
            <SelectTrigger><SelectValue placeholder="เลือก Device" /></SelectTrigger>
            <SelectContent>{devices.map((entry) => <SelectItem key={entry.id} value={entry.id}>{entry.name} ({entry.externalId})</SelectItem>)}</SelectContent>
          </Select>
        </label>
        <label className="ea-field">Start
          <TextInput
            type="datetime-local"
            className="ea-input"
            value={toDatetimeLocal(range.from)}
            onChange={(event) => {
              const from = new Date(event.target.value);
              if (!Number.isNaN(+from)) setRange((value) => ({ ...value, from }));
            }}
          />
        </label>
        <label className="ea-field">End
          <TextInput
            type="datetime-local"
            className="ea-input"
            value={toDatetimeLocal(range.to)}
            onChange={(event) => {
              const to = new Date(event.target.value);
              if (!Number.isNaN(+to)) setRange((value) => ({ ...value, to }));
            }}
          />
        </label>
        <div className="ea-field">ช่วงเวลา
          <div className="ea-presets">
            {PRESETS.map((preset) => (
              <Button variant="bare" key={preset.hours} type="button" className="ea-preset" onClick={() => setRange(rangeOfHours(preset.hours))}>
                {preset.label}
              </Button>
            ))}
          </div>
        </div>
        <label className="ea-toggle">
          <Checkbox checked={compare} onChange={setCompare} />
          เทียบกับช่วงก่อนหน้า
        </label>
      </div>

      {error && <FormMessage>{error}</FormMessage>}
      {current.truncated && (
        <p className="ts-truncated">ข้อมูลในช่วงนี้มากเกินไป แสดงเฉพาะส่วนล่าสุด — ลากบนกราฟเพื่อ zoom เข้าไปดูช่วงที่ต้องการ</p>
      )}

      {!deviceId ? (
        <div className="table-state">เลือก Plant และ Device เพื่อดูกราฟ</div>
      ) : (
        <>
          <KpiRow
            metric={metric}
            points={metricPoints}
            basePoints={compare ? basePoints : undefined}
            installedDcKw={plant?.installedDcKw ?? null}
          />

          <div className="ea-picker">
            <MultiSelect
              value={selectedKeys}
              onValueChange={setSelectedKeys}
              options={availableKeys.map((key) => {
                const meta = pointMeta(catalog, key);
                return { label: meta.displayName, value: key, unit: meta.unit, tag: meta.tag };
              })}
              placeholder="เลือก Parameter เพื่อ plot กราฟ"
              ariaLabel="เลือก Parameter เพื่อ plot กราฟ"
              disabled={availableKeys.length === 0}
            />
          </div>

          <div className="ea-analysis-tabs" role="tablist" aria-label="Analytics view">
            <Button variant={analysisView === "trend" ? "primary" : "secondary"} compact onClick={() => setAnalysisView("trend")}>Trend Viewer</Button>
            <Button variant={analysisView === "xy" ? "primary" : "secondary"} compact onClick={() => setAnalysisView("xy")}>XY Scatter</Button>
            <Button variant={analysisView === "solar" ? "primary" : "secondary"} compact onClick={() => setAnalysisView("solar")}>Solar Power Curve</Button>
          </div>

          {analysisView === "trend" ? (
            <section className="ea-panel">
              {selectedKeys.length === 0 ? (
                <div className="timeseries-empty"><ChartLine size={24} /><span>เลือก Parameter เพื่อ plot กราฟ</span></div>
              ) : (
                <TimeSeriesChart
                  series={series}
                  from={range.from.getTime()}
                  to={range.to.getTime()}
                  onZoom={(from, to) => setRange({ from, to })}
                  onResetZoom={() => setRange(rangeOfHours(24))}
                />
              )}
            </section>
          ) : (
            <section className="ea-panel">
              <ScatterAnalysis
                points={analysisPoints}
                xLabel={analysisXKey ? pointMeta(catalog, analysisXKey).displayName : "X"}
                yLabel={analysisYKey ? pointMeta(catalog, analysisYKey).displayName : "Y"}
                title={analysisView === "solar" ? "Solar Power Curve" : "XY Scatter Analysis"}
              />
            </section>
          )}

          <section className="ea-panel">
            <div className="ea-panel-head">
              <h3>พลังงานราย{barStep === "day" ? "วัน" : "ชั่วโมง"}</h3>
              {metric && <span className="ts-unit">kWh · จาก {metric.displayName}</span>}
            </div>
            <EnergyBars buckets={buckets} baseline={baseBuckets} step={barStep} />
          </section>

          {loading && <p className="ts-hint">กำลังโหลดข้อมูล…</p>}
        </>
      )}
    </div>
  );
}

function ScatterAnalysis({ points, xLabel, yLabel, title }: { points: ScatterPoint[]; xLabel: string; yLabel: string; title: string }) {
  if (points.length === 0) return <div className="timeseries-empty"><ChartLine size={24} /><span>เลือก Parameter ที่มี timestamp ตรงกันอย่างน้อย 2 ชุดเพื่อสร้าง {title}</span></div>;
  const width = 720;
  const height = 300;
  const margin = 34;
  const minX = Math.min(...points.map((point) => point.x));
  const maxX = Math.max(...points.map((point) => point.x));
  const minY = Math.min(...points.map((point) => point.y));
  const maxY = Math.max(...points.map((point) => point.y));
  const spanX = maxX - minX || 1;
  const spanY = maxY - minY || 1;
  const pointX = (value: number) => margin + ((value - minX) / spanX) * (width - margin * 2);
  const pointY = (value: number) => height - margin - ((value - minY) / spanY) * (height - margin * 2);
  return (
    <div className="xy-analysis" aria-label={title}>
      <div className="ea-panel-head"><h3>{title}</h3><span className="ts-unit">{xLabel} × {yLabel} · {points.length.toLocaleString()} points</span></div>
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`${title}: ${xLabel} versus ${yLabel}`} className="xy-analysis-chart">
        <line x1={margin} y1={height - margin} x2={width - margin} y2={height - margin} stroke="currentColor" opacity=".25" />
        <line x1={margin} y1={margin} x2={margin} y2={height - margin} stroke="currentColor" opacity=".25" />
        {points.map((point) => <circle key={`${point.t}-${point.x}-${point.y}`} cx={pointX(point.x)} cy={pointY(point.y)} r="3" fill="var(--accent)" opacity=".7" />)}
        <text x={width / 2} y={height - 8} textAnchor="middle">{xLabel}</text>
        <text x="12" y={height / 2} textAnchor="middle" transform={`rotate(-90 12 ${height / 2})`}>{yLabel}</text>
      </svg>
    </div>
  );
}

function KpiRow({
  metric,
  points,
  basePoints,
  installedDcKw,
}: {
  metric?: PointMeta;
  points: Point[];
  basePoints?: Point[];
  installedDcKw: number | null;
}) {
  if (!metric) {
    return <div className="table-state">ไม่พบ Parameter ที่เป็นกำลังไฟ (kW) หรือพลังงาน (kWh) จึงสรุปค่าไม่ได้</div>;
  }
  const isPower = classifyUnit(metric.unit) === "power";
  const totals = summarise(points, metric, isPower);
  const before = basePoints ? summarise(basePoints, metric, isPower) : undefined;

  const cards: Array<{ label: string; value: string; hint?: string; delta?: number | null }> = [
    {
      label: "พลังงานรวม",
      value: `${format(totals.kwh)} kWh`,
      hint: metric.displayName,
      delta: before ? percentChange(totals.kwh, before.kwh) : undefined,
    },
  ];
  if (isPower) {
    cards.push({
      label: "กำลังไฟสูงสุด",
      value: `${format(totals.peak)} ${metric.unit}`,
      hint: totals.peakAt ? peakFormat.format(new Date(totals.peakAt)) : undefined,
      delta: before ? percentChange(totals.peak, before.peak) : undefined,
    });
    cards.push({
      label: "กำลังไฟเฉลี่ย",
      value: `${format(totals.average)} ${metric.unit}`,
      delta: before ? percentChange(totals.average, before.average) : undefined,
    });
  }
  if (installedDcKw && installedDcKw > 0) {
    const yieldNow = totals.kwh / installedDcKw;
    cards.push({
      label: "Specific yield",
      value: `${format(yieldNow)} kWh/kWp`,
      hint: `ติดตั้ง ${format(installedDcKw)} kWp`,
      delta: before ? percentChange(yieldNow, before.kwh / installedDcKw) : undefined,
    });
  }

  return (
    <div className="ea-kpis">
      {cards.map((card) => (
        <div key={card.label} className="ea-kpi">
          <small>{card.label}</small>
          <strong>{card.value}</strong>
          <span className="ea-kpi-foot">
            <span className="ea-kpi-hint">{card.hint}</span>
            {card.delta != null && (
              <em className={card.delta >= 0 ? "ea-up" : "ea-down"}>
                {card.delta >= 0 ? "▲" : "▼"} {Math.abs(card.delta).toFixed(1)}%
              </em>
            )}
          </span>
        </div>
      ))}
    </div>
  );
}

const peakFormat = new Intl.DateTimeFormat("th-TH", { dateStyle: "short", timeStyle: "short" });

function summarise(points: Point[], metric: PointMeta, isPower: boolean) {
  const kwh = totalEnergyKWh(points, metric.unit) ?? 0;
  let peak = 0;
  let peakAt = 0;
  let sum = 0;
  for (const point of points) {
    if (point.v > peak) {
      peak = point.v;
      peakAt = point.t;
    }
    sum += point.v;
  }
  return { kwh, peak: isPower ? peak : 0, peakAt, average: isPower && points.length > 0 ? sum / points.length : 0 };
}

function percentChange(now: number, before: number) {
  if (!Number.isFinite(before) || before === 0) return null;
  return ((now - before) / Math.abs(before)) * 100;
}

function format(value: number) {
  const magnitude = Math.abs(value);
  const digits = magnitude >= 100 ? 0 : magnitude >= 1 ? 1 : 3;
  return value.toLocaleString(undefined, { minimumFractionDigits: digits, maximumFractionDigits: digits });
}

const barLabelFormat = {
  hour: new Intl.DateTimeFormat("th-TH", { hour: "2-digit", hour12: false }),
  day: new Intl.DateTimeFormat("th-TH", { day: "numeric", month: "short" }),
};

function EnergyBars({ buckets, baseline, step }: { buckets: Bucket[]; baseline: Bucket[]; step: "hour" | "day" }) {
  if (buckets.length === 0) return <div className="table-state">ไม่มีข้อมูลพอสำหรับสรุปพลังงาน</div>;
  const peak = Math.max(...buckets.map((bucket) => bucket.kwh), ...baseline.map((bucket) => bucket.kwh), 1);
  // Thin the labels out once the bars get narrow, otherwise they overlap into
  // an unreadable smear.
  const labelEvery = Math.ceil(buckets.length / 12);

  return (
    <div className="ea-bars" role="img" aria-label="กราฟแท่งพลังงาน">
      {buckets.map((bucket, index) => {
        const before = baseline[index];
        return (
          <div key={bucket.key} className="ea-bar-slot" title={`${bucket.key} · ${format(bucket.kwh)} kWh`}>
            <div className="ea-bar-stack">
              {before && <div className="ea-bar ea-bar-base" style={{ height: `${(before.kwh / peak) * 100}%` }} />}
              <div className="ea-bar" style={{ height: `${(bucket.kwh / peak) * 100}%` }} />
            </div>
            <small>{index % labelEvery === 0 ? barLabelFormat[step].format(new Date(bucket.start)) : ""}</small>
          </div>
        );
      })}
    </div>
  );
}
