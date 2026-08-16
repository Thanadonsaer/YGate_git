"use client";

import { ChartLine, Download, RefreshCw } from "lucide-react";
import { Checkbox, FormMessage, TextInput } from "../../components/ui/form";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  api,
  downloadBlob,
  errorMessage,
  toDatetimeLocal,
} from "../../lib/api";
import {
  fetchRanges,
  loadRegisterCatalogs,
  pointMeta,
  type PointMeta,
} from "../../lib/telemetry-history";
import {
  bucketEnergy,
  classifyUnit,
  fillBuckets,
  previousPeriod,
  seriesToCSV,
  sumBuckets,
  toSeries,
  totalEnergyKWh,
  valueTicks,
  type Bucket,
  type Point,
} from "../../lib/telemetry-math";
import {
  SERIES_COLORS,
  TimeSeriesChart,
  type ChartSeries,
} from "../../components/charts/time-series-chart";
import { MultiSelect } from "../../components/ui/multi-select";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "../../components/ui/select";
import { Button } from "../../components/ui/button";
import type { Device, Plant } from "../../lib/types";
import {
  buildScatterPoints,
  findSignalKey,
  type ScatterPoint,
} from "../../lib/xy-scatter";

const PRESETS = [
  { label: "24 ชม.", hours: 24 },
  { label: "7 วัน", hours: 24 * 7 },
  { label: "30 วัน", hours: 24 * 30 },
];

/** Past this span an hourly bar chart is unreadable, so bars roll up to days. */
const DAILY_BARS_AFTER_MS = 2 * 24 * 60 * 60 * 1000;

/**
 * How long the device picker has to sit still before anything is fetched.
 *
 * Picking devices is a burst of clicks in one open dropdown, and every click
 * changes deviceIds -- which used to cancel and restart the catalog *and*
 * history load for every device already picked. Choosing six devices therefore
 * paid for six full reloads, hundreds of requests, nearly all of them aborted
 * by the next click and logged as failures. Waiting for the burst to settle
 * turns that back into one load.
 */
const SELECT_SETTLE_MS = 400;

function rangeOfHours(hours: number) {
  const to = new Date();
  return { from: new Date(to.getTime() - hours * 3_600_000), to };
}

/** Points per point-key, per device id. */
type SeriesByDevice = Record<string, Record<string, Point[]>>;
type Loaded = { byDevice: SeriesByDevice; truncated: boolean };

export function EnergyAnalysisPage() {
  const [plants, setPlants] = useState<Plant[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [plantId, setPlantId] = useState("");
  const [deviceIds, setDeviceIds] = useState<string[]>([]);
  // What the picker shows vs. what has been fetched: everything below the
  // debounce reads activeDeviceIds, so mid-burst clicks never start a load.
  const [activeDeviceIds, setActiveDeviceIds] = useState<string[]>([]);
  const [range, setRange] = useState(() => rangeOfHours(24));
  const [compare, setCompare] = useState(false);
  const [analysisView, setAnalysisView] = useState<"trend" | "xy" | "solar">(
    "trend",
  );
  const [catalogs, setCatalogs] = useState<
    Record<string, Record<string, PointMeta>>
  >({});
  const [current, setCurrent] = useState<Loaded>({
    byDevice: {},
    truncated: false,
  });
  const [baseline, setBaseline] = useState<SeriesByDevice>({});
  const [selectedKeys, setSelectedKeys] = useState<string[]>([]);
  const [reloadToken, setReloadToken] = useState(0);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const plant = plants.find((entry) => entry.id === plantId);

  useEffect(() => {
    const timer = setTimeout(() => setActiveDeviceIds(deviceIds), SELECT_SETTLE_MS);
    return () => clearTimeout(timer);
  }, [deviceIds]);

  const selectedDevices = useMemo(
    () =>
      activeDeviceIds
        .map((id) => devices.find((device) => device.id === id))
        .filter((device): device is Device => Boolean(device)),
    [activeDeviceIds, devices],
  );

  useEffect(() => {
    void api("/api/v1/plants")
      .then(async (response) => {
        if (!response.ok) throw new Error("ไม่สามารถโหลด Plant ได้");
        setPlants((await response.json()) as Plant[]);
      })
      .catch((cause: unknown) => setError(errorMessage(cause)));
  }, []);

  useEffect(() => {
    setDeviceIds([]);
    // Cleared here as well as by the debounce: for those 400ms the old plant's
    // device ids would otherwise be fetched against the new plant, and every
    // one of them 404s.
    setActiveDeviceIds([]);
    setDevices([]);
    if (!plantId) return;
    const controller = new AbortController();
    void api(`/api/v1/plants/${encodeURIComponent(plantId)}/devices`, {
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) throw new Error("ไม่สามารถโหลด Device ได้");
        const list = (await response.json()) as Device[];
        setDevices(list);
        // Land on the first device rather than an empty chart, the way the
        // single-select version did.
        setDeviceIds(list[0] ? [list[0].id] : []);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [plantId]);

  useEffect(() => {
    setCatalogs({});
    if (!plantId || selectedDevices.length === 0) return;
    const controller = new AbortController();
    void loadRegisterCatalogs(plantId, selectedDevices, controller.signal)
      .then(setCatalogs)
      .catch(() => {
        if (!controller.signal.aborted)
          setError("ไม่สามารถโหลด Metadata ของ Parameter ได้");
      });
    return () => controller.abort();
  }, [plantId, selectedDevices]);

  useEffect(() => {
    if (!plantId || activeDeviceIds.length === 0) return;
    if (range.from >= range.to) {
      setError("ช่วงเวลาไม่ถูกต้อง (start ต้องอยู่ก่อน end)");
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setError("");
    void (async () => {
      try {
        const pages = await fetchRanges(
          plantId,
          activeDeviceIds,
          range.from,
          range.to,
          controller.signal,
        );
        setCurrent({
          byDevice: Object.fromEntries(
            activeDeviceIds.map((id, index) => [
              id,
              toSeries(pages[index].readings),
            ]),
          ),
          truncated: pages.some((page) => page.truncated),
        });
        if (compare) {
          const window = previousPeriod(range.from, range.to);
          const before = await fetchRanges(
            plantId,
            activeDeviceIds,
            window.from,
            window.to,
            controller.signal,
          );
          setBaseline(
            Object.fromEntries(
              activeDeviceIds.map((id, index) => [
                id,
                toSeries(before[index].readings),
              ]),
            ),
          );
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
  }, [plantId, activeDeviceIds, range, compare, reloadToken]);

  // One register can appear on several devices; the first catalog that knows it
  // supplies the display name, since they are the same register either way.
  const metaOf = useCallback(
    (key: string) => {
      for (const id of activeDeviceIds) {
        const meta = catalogs[id]?.[key];
        if (meta) return meta;
      }
      return pointMeta({}, key);
    },
    [catalogs, activeDeviceIds],
  );

  // Union, not intersection: a register only one of the selected devices
  // reports is still worth plotting.
  const availableKeys = useMemo(() => {
    const keys = new Set<string>();
    for (const id of activeDeviceIds) {
      for (const key of Object.keys(current.byDevice[id] ?? {})) {
        if (catalogs[id]?.[key]?.isEnabled !== false) keys.add(key);
      }
    }
    return [...keys].sort((a, b) =>
      metaOf(a).displayName.localeCompare(metaOf(b).displayName),
    );
  }, [current.byDevice, catalogs, activeDeviceIds, metaOf]);

  // Default to whatever can actually be analysed -- a power or energy register
  // -- instead of the alphabetically first key, which is usually a voltage.
  useEffect(() => {
    setSelectedKeys((currentKeys) => {
      const kept = currentKeys.filter((key) => availableKeys.includes(key));
      if (kept.length > 0)
        return kept.length === currentKeys.length ? currentKeys : kept;
      const preferred = availableKeys.filter(
        (key) => classifyUnit(metaOf(key).unit) !== "other",
      );
      return (preferred.length > 0 ? preferred : availableKeys).slice(0, 3);
    });
  }, [availableKeys, metaOf]);

  const metricKey = useMemo(() => {
    const candidates = selectedKeys.length > 0 ? selectedKeys : availableKeys;
    return (
      candidates.find((key) => classifyUnit(metaOf(key).unit) === "power") ??
      candidates.find((key) => classifyUnit(metaOf(key).unit) === "energy")
    );
  }, [selectedKeys, availableKeys, metaOf]);

  const metric = metricKey ? metaOf(metricKey) : undefined;
  const metricByDevice = useMemo(
    () =>
      metricKey
        ? activeDeviceIds.map((id) => current.byDevice[id]?.[metricKey] ?? [])
        : [],
    [metricKey, activeDeviceIds, current.byDevice],
  );
  const baseByDevice = useMemo(
    () =>
      metricKey
        ? activeDeviceIds.map((id) => baseline[id]?.[metricKey] ?? [])
        : [],
    [metricKey, activeDeviceIds, baseline],
  );

  // The scatter views correlate two registers sample-for-sample, which only
  // means anything within one device, so they read the first one selected.
  const scatterDeviceId = activeDeviceIds[0] ?? "";
  const scatterSeries = current.byDevice[scatterDeviceId] ?? {};
  const solarXKey =
    findSignalKey(availableKeys, /irradiance|irradiation|sun/i) ??
    selectedKeys[0];
  const solarYKey =
    findSignalKey(availableKeys, /active.?power|power.?ac|ac.?power/i) ??
    selectedKeys[1];
  const analysisXKey = analysisView === "solar" ? solarXKey : selectedKeys[0];
  const analysisYKey = analysisView === "solar" ? solarYKey : selectedKeys[1];
  const analysisPoints =
    analysisXKey && analysisYKey
      ? buildScatterPoints(
        scatterSeries[analysisXKey] ?? [],
        scatterSeries[analysisYKey] ?? [],
      )
      : [];
  const barStep: "hour" | "day" =
    range.to.getTime() - range.from.getTime() > DAILY_BARS_AFTER_MS
      ? "day"
      : "hour";

  // Zero-filled before summing so every device contributes on the same time
  // axis, and so the bars line up with the labels under them.
  const buckets = useMemo(
    () =>
      metric
        ? fillBuckets(
          sumBuckets(
            metricByDevice.map((points) =>
              bucketEnergy(points, metric.unit, barStep),
            ),
          ),
          range.from.getTime(),
          range.to.getTime(),
          barStep,
        )
        : [],
    [metricByDevice, metric, barStep, range],
  );
  const baseBuckets = useMemo(() => {
    if (!metric || !compare) return [];
    const window = previousPeriod(range.from, range.to);
    return fillBuckets(
      sumBuckets(
        baseByDevice.map((points) =>
          bucketEnergy(points, metric.unit, barStep),
        ),
      ),
      window.from.getTime(),
      window.to.getTime(),
      barStep,
    );
  }, [baseByDevice, metric, barStep, compare, range]);

  const series = useMemo<ChartSeries[]>(() => {
    const entries: ChartSeries[] = [];
    for (const key of selectedKeys) {
      const meta = metaOf(key);
      for (const device of selectedDevices) {
        const points = current.byDevice[device.id]?.[key];
        if (!points || points.length === 0) continue;
        entries.push({
          key: `${device.id}::${key}`,
          // One device is the common case; naming it on every line then is
          // just noise on the legend.
          label:
            selectedDevices.length > 1
              ? `${device.name} · ${meta.displayName}`
              : meta.displayName,
          unit: meta.unit,
          decimals: meta.decimals,
          color: SERIES_COLORS[entries.length % SERIES_COLORS.length],
          points,
        });
      }
    }
    return entries;
  }, [selectedKeys, selectedDevices, current.byDevice, metaOf]);

  const exportCSV = useCallback(() => {
    if (series.length === 0) return;
    const flat: Record<string, Point[]> = {};
    const columns = series.map((entry) => {
      flat[entry.key] = entry.points;
      return {
        key: entry.key,
        header: entry.unit ? `${entry.label} (${entry.unit})` : entry.label,
      };
    });
    const csv = seriesToCSV(columns, flat);
    const stamp = toDatetimeLocal(range.from)
      .replace(/[-:]/g, "")
      .replace("T", "-");
    const name =
      selectedDevices.length === 1
        ? selectedDevices[0].externalId
        : `${plant?.code ?? "plant"}-${selectedDevices.length}devices`;
    // Excel needs the BOM to read the Thai display names as UTF-8.
    downloadBlob(
      new Blob([`﻿${csv}`], { type: "text/csv;charset=utf-8" }),
      `ygate-${name}-${stamp}.csv`,
    );
  }, [series, selectedDevices, plant, range.from]);

  return (
    <div className="content energy-analysis-content">
      <div className="section-heading">
        <div>
          <p>Inverter telemetry</p>
          <h2>Energy Analysis</h2>
        </div>
        <div className="heading-actions">
          <Button
            variant="icon"
            onClick={exportCSV}
            disabled={series.length === 0}
            title="ดาวน์โหลด CSV"
            aria-label="ดาวน์โหลด CSV"
          >
            <Download size={18} />
          </Button>
          <Button
            variant="icon"
            onClick={() => setReloadToken((token) => token + 1)}
            disabled={loading || deviceIds.length === 0}
            title="รีเฟรช"
            aria-label="รีเฟรช"
          >
            <RefreshCw size={18} />
          </Button>
        </div>
      </div>

      {/* Scope (what) on the first row, time range (when) on the second, so the
          two questions the filter answers stop being one wrapping flex line. */}
      <section className="ea-filters">
        <div className="ea-filter-row">
          <label className="ea-field">
            Plant
            <Select value={plantId} onValueChange={setPlantId}>
              <SelectTrigger>
                <SelectValue placeholder="เลือก Plant" />
              </SelectTrigger>
              <SelectContent>
                {plants.map((entry) => (
                  <SelectItem key={entry.id} value={entry.id}>
                    {entry.name} ({entry.code})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </label>
          <label className="ea-field ea-field-wide">
            Device{" "}
            <MultiSelect
              value={deviceIds}
              onValueChange={setDeviceIds}
              options={devices.map((entry) => ({
                label: entry.name,
                value: entry.id,
                tag: entry.externalId,
              }))}
              placeholder={plantId ? "เลือก Device" : "เลือก Plant ก่อน"}
              ariaLabel="เลือก Device"
              disabled={devices.length === 0}
            />

          </label>
        </div>

        <div className="ea-filter-row">
          <label className="ea-field">
            Start
            <TextInput
              type="datetime-local"
              className="ea-input"
              value={toDatetimeLocal(range.from)}
              onChange={(event) => {
                const from = new Date(event.target.value);
                if (!Number.isNaN(+from))
                  setRange((value) => ({ ...value, from }));
              }}
            />
          </label>
          <label className="ea-field">
            End
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
          <div className="ea-field">
            ช่วงเวลา
            <div className="ea-presets">
              {PRESETS.map((preset) => (
                <Button
                  variant="bare"
                  key={preset.hours}
                  type="button"
                  className="ea-preset"
                  onClick={() => setRange(rangeOfHours(preset.hours))}
                >
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
      </section>

      {error && <FormMessage>{error}</FormMessage>}
      {/* Also while the picker is still settling, otherwise the 400ms before a
          fetch starts reads as the click having done nothing. */}
      {(loading || deviceIds.join() !== activeDeviceIds.join()) && <div className="analytics-loading" role="status" aria-live="polite"><RefreshCw className="spin" size={20} /> กำลังโหลดข้อมูล...</div>}
      {current.truncated && (
        <p className="ts-truncated">
          ข้อมูลในช่วงนี้มากเกินไป แสดงเฉพาะส่วนล่าสุด — ลากบนกราฟเพื่อ zoom
          เข้าไปดูช่วงที่ต้องการ
        </p>
      )}

      {/* Gated on the settled list, not the picker: keying this off deviceIds
          flashed "no power parameter found" for the 400ms between a click and
          its data, which reads as a failure rather than as loading. */}
      {activeDeviceIds.length === 0 ? (
        <div className="table-state">เลือก Plant และ Device เพื่อดูกราฟ</div>
      ) : (
        <>
          <KpiRow
            metric={metric}
            pointsByDevice={metricByDevice}
            basePointsByDevice={compare ? baseByDevice : undefined}
            installedDcKw={plant?.installedDcKw ?? null}
            deviceCount={selectedDevices.length}
          />

          <div className="ea-picker">
            <MultiSelect
              value={selectedKeys}
              onValueChange={setSelectedKeys}
              options={availableKeys.map((key) => {
                const meta = metaOf(key);
                return {
                  label: meta.displayName,
                  value: key,
                  unit: meta.unit,
                  tag: meta.tag,
                };
              })}
              placeholder="เลือก Parameter เพื่อ plot กราฟ"
              ariaLabel="เลือก Parameter เพื่อ plot กราฟ"
              disabled={availableKeys.length === 0}
            />
          </div>

          <div
            className="ea-analysis-tabs"
            role="tablist"
            aria-label="Analytics view"
          >
            <Button
              variant={analysisView === "trend" ? "primary" : "secondary"}
              compact
              onClick={() => setAnalysisView("trend")}
            >
              Trend Viewer
            </Button>
            <Button
              variant={analysisView === "xy" ? "primary" : "secondary"}
              compact
              onClick={() => setAnalysisView("xy")}
            >
              XY Scatter
            </Button>
            <Button
              variant={analysisView === "solar" ? "primary" : "secondary"}
              compact
              onClick={() => setAnalysisView("solar")}
            >
              Solar Power Curve
            </Button>
          </div>

          {analysisView === "trend" ? (
            <section className="ea-panel">
              {series.length === 0 ? (
                <div className="timeseries-empty">
                  <ChartLine size={24} />
                  <span>เลือก Parameter เพื่อ plot กราฟ</span>
                </div>
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
                xLabel={analysisXKey ? metaOf(analysisXKey).displayName : "X"}
                yLabel={analysisYKey ? metaOf(analysisYKey).displayName : "Y"}
                title={
                  analysisView === "solar"
                    ? "Solar Power Curve"
                    : "XY Scatter Analysis"
                }
                deviceName={
                  selectedDevices.length > 1
                    ? selectedDevices[0]?.name
                    : undefined
                }
              />
            </section>
          )}

          <section className="ea-panel">
            <div className="ea-panel-head">
              <h3>พลังงานราย{barStep === "day" ? "วัน" : "ชั่วโมง"}</h3>
              {metric && (
                <span className="ts-unit">
                  kWh · จาก {metric.displayName}
                  {selectedDevices.length > 1
                    ? ` · รวม ${selectedDevices.length} เครื่อง`
                    : ""}
                </span>
              )}
            </div>
            <EnergyBars
              buckets={buckets}
              baseline={baseBuckets}
              step={barStep}
            />
          </section>

        </>
      )}
    </div>
  );
}

function ScatterAnalysis({
  points,
  xLabel,
  yLabel,
  title,
  deviceName,
}: {
  points: ScatterPoint[];
  xLabel: string;
  yLabel: string;
  title: string;
  deviceName?: string;
}) {
  if (points.length === 0)
    return (
      <div className="timeseries-empty">
        <ChartLine size={24} />
        <span>
          เลือก Parameter ที่มี timestamp ตรงกันอย่างน้อย 2 ชุดเพื่อสร้าง{" "}
          {title}
        </span>
      </div>
    );
  const width = 720;
  const height = 300;
  const margin = 34;
  const minX = Math.min(...points.map((point) => point.x));
  const maxX = Math.max(...points.map((point) => point.x));
  const minY = Math.min(...points.map((point) => point.y));
  const maxY = Math.max(...points.map((point) => point.y));
  const spanX = maxX - minX || 1;
  const spanY = maxY - minY || 1;
  const pointX = (value: number) =>
    margin + ((value - minX) / spanX) * (width - margin * 2);
  const pointY = (value: number) =>
    height - margin - ((value - minY) / spanY) * (height - margin * 2);
  return (
    <div className="xy-analysis" aria-label={title}>
      <div className="ea-panel-head">
        <h3>{title}</h3>
        {/* Correlating two registers only means anything within one device, so
            say which one when several are selected. */}
        <span className="ts-unit">
          {deviceName ? `${deviceName} · ` : ""}
          {xLabel} × {yLabel} · {points.length.toLocaleString()} points
        </span>
      </div>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label={`${title}: ${xLabel} versus ${yLabel}`}
        className="xy-analysis-chart"
      >
        <line
          x1={margin}
          y1={height - margin}
          x2={width - margin}
          y2={height - margin}
          stroke="currentColor"
          opacity=".25"
        />
        <line
          x1={margin}
          y1={margin}
          x2={margin}
          y2={height - margin}
          stroke="currentColor"
          opacity=".25"
        />
        {points.map((point) => (
          <circle
            key={`${point.t}-${point.x}-${point.y}`}
            cx={pointX(point.x)}
            cy={pointY(point.y)}
            r="3"
            fill="var(--accent)"
            opacity=".7"
          />
        ))}
        <text x={width / 2} y={height - 8} textAnchor="middle">
          {xLabel}
        </text>
        <text
          x="12"
          y={height / 2}
          textAnchor="middle"
          transform={`rotate(-90 12 ${height / 2})`}
        >
          {yLabel}
        </text>
      </svg>
    </div>
  );
}

function KpiRow({
  metric,
  pointsByDevice,
  basePointsByDevice,
  installedDcKw,
  deviceCount,
}: {
  metric?: PointMeta;
  pointsByDevice: Point[][];
  basePointsByDevice?: Point[][];
  installedDcKw: number | null;
  deviceCount: number;
}) {
  if (!metric) {
    return (
      <div className="table-state">
        ไม่พบ Parameter ที่เป็นกำลังไฟ (kW) หรือพลังงาน (kWh) จึงสรุปค่าไม่ได้
      </div>
    );
  }
  const isPower = classifyUnit(metric.unit) === "power";
  const totals = summarise(pointsByDevice, metric, isPower);
  const before = basePointsByDevice
    ? summarise(basePointsByDevice, metric, isPower)
    : undefined;
  const scope =
    deviceCount > 1
      ? `${metric.displayName} · รวม ${deviceCount} เครื่อง`
      : metric.displayName;

  const cards: Array<{
    label: string;
    value: string;
    hint?: string;
    delta?: number | null;
  }> = [
      {
        label: "พลังงานรวม",
        value: `${format(totals.kwh)} kWh`,
        hint: scope,
        delta: before ? percentChange(totals.kwh, before.kwh) : undefined,
      },
    ];
  if (isPower) {
    cards.push({
      label: deviceCount > 1 ? "กำลังไฟสูงสุด (รวม)" : "กำลังไฟสูงสุด",
      value: `${format(totals.peak)} ${metric.unit}`,
      hint: totals.peakAt
        ? peakFormat.format(new Date(totals.peakAt))
        : undefined,
      delta: before ? percentChange(totals.peak, before.peak) : undefined,
    });
    cards.push({
      label: deviceCount > 1 ? "กำลังไฟเฉลี่ย (รวม)" : "กำลังไฟเฉลี่ย",
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
      delta: before
        ? percentChange(yieldNow, before.kwh / installedDcKw)
        : undefined,
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

const peakFormat = new Intl.DateTimeFormat("th-TH", {
  dateStyle: "short",
  timeStyle: "short",
});

/**
 * Plant-level totals across the selected devices: energy adds up, and so does
 * instantaneous power -- but only sample-for-sample. Peak is therefore the
 * highest *combined* output at one moment, not the sum of each device's own
 * peak, which would overstate it whenever they peak at different times.
 */
function summarise(
  pointsByDevice: Point[][],
  metric: PointMeta,
  isPower: boolean,
) {
  const kwh = pointsByDevice.reduce(
    (total, points) => total + (totalEnergyKWh(points, metric.unit) ?? 0),
    0,
  );
  if (!isPower) return { kwh, peak: 0, peakAt: 0, average: 0 };

  const combined = new Map<number, number>();
  for (const points of pointsByDevice) {
    for (const point of points)
      combined.set(point.t, (combined.get(point.t) ?? 0) + point.v);
  }
  let peak = 0;
  let peakAt = 0;
  let sum = 0;
  for (const [t, value] of combined) {
    if (value > peak) {
      peak = value;
      peakAt = t;
    }
    sum += value;
  }
  return {
    kwh,
    peak,
    peakAt,
    average: combined.size > 0 ? sum / combined.size : 0,
  };
}

function percentChange(now: number, before: number) {
  if (!Number.isFinite(before) || before === 0) return null;
  return ((now - before) / Math.abs(before)) * 100;
}

function format(value: number) {
  const magnitude = Math.abs(value);
  const digits = magnitude >= 100 ? 0 : magnitude >= 1 ? 1 : 3;
  return value.toLocaleString(undefined, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  });
}

const barLabelFormat = {
  hour: new Intl.DateTimeFormat("th-TH", { hour: "2-digit", hour12: false }),
  day: new Intl.DateTimeFormat("th-TH", { day: "numeric", month: "short" }),
};

/**
 * Buckets arrive zero-filled and in order (fillBuckets), so slot N really is
 * the Nth hour/day of the range and the previous-period overlay can be read
 * off by index. Both of those were wrong while bucketEnergy's sparse output
 * was rendered directly: empty hours collapsed so the bars drifted away from
 * the labels beneath them, and the overlay compared whichever buckets happened
 * to land on the same array position.
 */
function EnergyBars({
  buckets,
  baseline,
  step,
}: {
  buckets: Bucket[];
  baseline: Bucket[];
  step: "hour" | "day";
}) {
  if (buckets.length === 0)
    return <div className="table-state">ไม่มีข้อมูลพอสำหรับสรุปพลังงาน</div>;
  const peak = Math.max(
    ...buckets.map((bucket) => bucket.kwh),
    ...baseline.map((bucket) => bucket.kwh),
    0,
  );
  const ticks = valueTicks(0, peak || 1, 4);
  // Scale to the top gridline, not to the tallest bar, so a bar's height can
  // actually be read off the axis beside it.
  const top = Math.max(peak, ticks[ticks.length - 1] ?? 0, 1);
  // Thin the labels out once the bars get narrow, otherwise they overlap into
  // an unreadable smear.
  const labelEvery = Math.ceil(buckets.length / 12);

  return (
    <div className="ea-bars-frame">
      <div className="ea-bars-axis" aria-hidden="true">
        {ticks.map((tick) => (
          <span key={tick} style={{ bottom: `${(tick / top) * 100}%` }}>
            {format(tick)}
          </span>
        ))}
      </div>
      <div className="ea-bars" role="img" aria-label="กราฟแท่งพลังงาน">
        {ticks.map((tick) => (
          <i
            key={tick}
            className="ea-bars-grid"
            style={{ bottom: `${(tick / top) * 100}%` }}
          />
        ))}
        {buckets.map((bucket, index) => {
          const before = baseline[index];
          return (
            <div
              key={bucket.key}
              className="ea-bar-slot"
              title={`${bucket.key} · ${format(bucket.kwh)} kWh${before ? ` (ก่อนหน้า ${format(before.kwh)} kWh)` : ""}`}
            >
              {before && (
                <div
                  className="ea-bar ea-bar-base"
                  style={{ height: `${(before.kwh / top) * 100}%` }}
                />
              )}
              <div
                className="ea-bar"
                style={{ height: `${(bucket.kwh / top) * 100}%` }}
              />
            </div>
          );
        })}
      </div>
      <div className="ea-bars-labels" aria-hidden="true">
        {buckets.map((bucket, index) => (
          <small key={bucket.key}>
            {index % labelEvery === 0
              ? barLabelFormat[step].format(new Date(bucket.start))
              : ""}
          </small>
        ))}
      </div>
    </div>
  );
}
