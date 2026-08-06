"use client";

import { ChartLine, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { api, errorMessage } from "../../lib/api";
import type { Device, Plant, TelemetryHistoryPage } from "../../lib/types";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { Button } from "../../components/ui/button";

const SERIES_COLORS = ["#2f7fe0", "#e0742f", "#2fa85a", "#a84dd6", "#d63f5c", "#4dbfd6"];

type SeriesPoint = { at: string; value: number };

function isEnergyLikeKey(key: string) {
  return /energy|power|kwh|kw\b/i.test(key);
}

function toDatetimeLocal(date: Date) {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function defaultRange() {
  const to = new Date();
  const from = new Date(to.getTime() - 24 * 60 * 60 * 1000);
  return { from: toDatetimeLocal(from), to: toDatetimeLocal(to) };
}

export function EnergyAnalysisPage() {
  const [plants, setPlants] = useState<Plant[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [plantId, setPlantId] = useState("");
  const [deviceId, setDeviceId] = useState("");
  const [range, setRange] = useState(defaultRange);
  const [pointKeys, setPointKeys] = useState<string[]>([]);
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set());
  const [seriesByKey, setSeriesByKey] = useState<Record<string, SeriesPoint[]>>({});
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    void api("/api/v1/plants").then(async (response) => {
      if (!response.ok) throw new Error("ไม่สามารถโหลด Plant ได้");
      setPlants((await response.json()) as Plant[]);
    }).catch((cause: unknown) => setError(errorMessage(cause)));
  }, []);

  useEffect(() => {
    setDeviceId("");
    setDevices([]);
    if (!plantId) return;
    const controller = new AbortController();
    void api(`/api/v1/plants/${encodeURIComponent(plantId)}/devices`, { signal: controller.signal }).then(async (response) => {
      if (!response.ok) throw new Error("ไม่สามารถโหลด Device ได้");
      setDevices((await response.json()) as Device[]);
    }).catch((cause: unknown) => { if (!controller.signal.aborted) setError(errorMessage(cause)); });
    return () => controller.abort();
  }, [plantId]);

  const loadHistory = useCallback(async () => {
    if (!plantId || !deviceId) return;
    const from = new Date(range.from);
    const to = new Date(range.to);
    if (Number.isNaN(+from) || Number.isNaN(+to) || from >= to) {
      setError("ช่วงเวลาไม่ถูกต้อง (start ต้องอยู่ก่อน end)");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const query = new URLSearchParams({ from: from.toISOString(), to: to.toISOString(), limit: "500" });
      const response = await api(`/api/v1/plants/${encodeURIComponent(plantId)}/devices/${encodeURIComponent(deviceId)}/telemetry/history?${query}`);
      if (!response.ok) throw new Error(response.status === 404 ? "ไม่พบ Plant หรือ Device" : "ไม่สามารถโหลดข้อมูล telemetry ได้");
      const page = (await response.json()) as TelemetryHistoryPage;
      const readings = [...page.data].reverse();
      const keys = new Set<string>();
      const byKey: Record<string, SeriesPoint[]> = {};
      for (const reading of readings) {
        for (const [key, value] of Object.entries(reading.dataItemMap)) {
          if (!Number.isFinite(value)) continue;
          keys.add(key);
          (byKey[key] ??= []).push({ at: reading.observedAt, value });
        }
      }
      const sortedKeys = [...keys].sort();
      setPointKeys(sortedKeys);
      setSeriesByKey(byKey);
      setSelectedKeys((current) => {
        if (current.size > 0) return new Set([...current].filter((key) => keys.has(key)));
        const preferred = sortedKeys.filter(isEnergyLikeKey).slice(0, 3);
        return new Set(preferred.length > 0 ? preferred : sortedKeys.slice(0, 1));
      });
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, [plantId, deviceId, range]);

  useEffect(() => { void loadHistory(); }, [loadHistory]);

  function toggleKey(key: string) {
    setSelectedKeys((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  }

  const activeKeys = pointKeys.filter((key) => selectedKeys.has(key));

  return (
    <div className="content energy-analysis-content">
      <div className="section-heading">
        <div><p>Inverter telemetry</p><h2>Energy Analysis</h2></div>
        <div className="heading-actions">
          <Button variant="icon" onClick={() => void loadHistory()} disabled={loading || !deviceId} title="รีเฟรช" aria-label="รีเฟรช"><RefreshCw size={18} /></Button>
        </div>
      </div>

      <div className="energy-analysis-filters">
        <label className="grid gap-1.5 text-xs font-bold text-ink">Plant
          <Select value={plantId} onValueChange={setPlantId}>
            <SelectTrigger><SelectValue placeholder="เลือก Plant" /></SelectTrigger>
            <SelectContent>{plants.map((plant) => <SelectItem key={plant.id} value={plant.id}>{plant.name} ({plant.code})</SelectItem>)}</SelectContent>
          </Select>
        </label>
        <label className="grid gap-1.5 text-xs font-bold text-ink">Device
          <Select value={deviceId} onValueChange={setDeviceId} disabled={!plantId}>
            <SelectTrigger><SelectValue placeholder="เลือก Device" /></SelectTrigger>
            <SelectContent>{devices.map((device) => <SelectItem key={device.id} value={device.id}>{device.name} ({device.externalId})</SelectItem>)}</SelectContent>
          </Select>
        </label>
        <label className="grid gap-1.5 text-xs font-bold text-ink">Start
          <input type="datetime-local" className="h-10 rounded-[var(--radius-sm)] border border-line px-3 text-sm" value={range.from} onChange={(event) => setRange((current) => ({ ...current, from: event.target.value }))} />
        </label>
        <label className="grid gap-1.5 text-xs font-bold text-ink">End
          <input type="datetime-local" className="h-10 rounded-[var(--radius-sm)] border border-line px-3 text-sm" value={range.to} onChange={(event) => setRange((current) => ({ ...current, to: event.target.value }))} />
        </label>
      </div>

      {error && <p className="form-message error">{error}</p>}

      {!deviceId ? (
        <div className="table-state">เลือก Plant และ Device เพื่อดูกราฟ</div>
      ) : (
        <div className="energy-analysis-body">
          <div className="energy-analysis-keys">
            {pointKeys.length === 0 && !loading && <div className="table-state">ไม่มีข้อมูล telemetry ในช่วงเวลานี้</div>}
            {pointKeys.map((key) => {
              const index = pointKeys.indexOf(key);
              return (
                <label key={key} className="energy-analysis-key">
                  <input type="checkbox" checked={selectedKeys.has(key)} onChange={() => toggleKey(key)} />
                  <span className="energy-analysis-swatch" style={{ background: SERIES_COLORS[index % SERIES_COLORS.length] }} />
                  <span>{key}</span>
                </label>
              );
            })}
          </div>

          <div className="energy-analysis-chart-panel">
            {activeKeys.length === 0 ? (
              <div className="timeseries-empty"><ChartLine size={24} /><span>เลือก point key เพื่อ plot กราฟ</span></div>
            ) : (
              <MultiSeriesChart pointKeys={activeKeys} seriesByKey={seriesByKey} />
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function MultiSeriesChart({ pointKeys, seriesByKey }: { pointKeys: string[]; seriesByKey: Record<string, SeriesPoint[]> }) {
  return (
    <>
      <svg className="timeseries-chart energy-analysis-chart" viewBox="0 0 100 40" preserveAspectRatio="none" role="img" aria-label="กราฟพลังงาน">
        <line x1="0" y1="36" x2="100" y2="36" />
        {pointKeys.map((key, seriesIndex) => {
          const points = seriesByKey[key] ?? [];
          if (points.length === 0) return null;
          const values = points.map((point) => point.value);
          const minimum = Math.min(...values);
          const maximum = Math.max(...values);
          const range = maximum - minimum || 1;
          const polyline = points.map((point, index) => `${points.length === 1 ? 50 : index * 100 / (points.length - 1)},${36 - (point.value - minimum) * 32 / range}`).join(" ");
          return <polyline key={key} points={polyline} style={{ stroke: SERIES_COLORS[seriesIndex % SERIES_COLORS.length] }} />;
        })}
      </svg>
      <div className="energy-analysis-legend">
        {pointKeys.map((key, seriesIndex) => {
          const points = seriesByKey[key] ?? [];
          const values = points.map((point) => point.value);
          const minimum = values.length ? Math.min(...values) : 0;
          const maximum = values.length ? Math.max(...values) : 0;
          const latest = values.length ? values[values.length - 1] : 0;
          return (
            <div key={key} className="energy-analysis-legend-item">
              <span className="energy-analysis-swatch" style={{ background: SERIES_COLORS[seriesIndex % SERIES_COLORS.length] }} />
              <strong>{key}</strong>
              <span>Min {minimum.toFixed(2)}</span>
              <span>Max {maximum.toFixed(2)}</span>
              <span>Latest {latest.toFixed(2)}</span>
            </div>
          );
        })}
      </div>
    </>
  );
}
