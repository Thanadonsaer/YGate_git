"use client";

import { useEffect, useMemo, useState } from "react";
import { FormMessage, TextInput } from "../../components/ui/form";
import { errorMessage, toDatetimeLocal } from "../../lib/api";
import { fetchRange, pointMeta, type PointMeta } from "../../lib/telemetry-history";
import { toSeries, type Point } from "../../lib/telemetry-math";
import { SERIES_COLORS, TimeSeriesChart, type ChartSeries } from "../../components/charts/time-series-chart";
import { MultiSelect } from "../../components/ui/multi-select";
import type { Device, Plant } from "../../lib/types";

function defaultRange() {
  const to = new Date();
  return { from: new Date(to.getTime() - 24 * 60 * 60 * 1000), to };
}

export function DeviceHistoryChart({
  plant,
  device,
  catalog,
  availableKeys,
}: {
  plant: Plant;
  device: Device;
  catalog: Record<string, PointMeta>;
  availableKeys: string[];
}) {
  const [selectedKeys, setSelectedKeys] = useState<string[]>(() => availableKeys.slice(0, 1));
  const [range, setRange] = useState(defaultRange);
  const [seriesByKey, setSeriesByKey] = useState<Record<string, Point[]>>({});
  const [truncated, setTruncated] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    setSelectedKeys((current) => {
      const kept = current.filter((key) => availableKeys.includes(key));
      return kept.length === current.length ? current : kept;
    });
  }, [availableKeys]);

  useEffect(() => {
    if (range.from >= range.to) {
      setError("ช่วงเวลาไม่ถูกต้อง (start ต้องอยู่ก่อน end)");
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setError("");
    void (async () => {
      try {
        const page = await fetchRange(plant.id, device.id, range.from, range.to, controller.signal);
        setSeriesByKey(toSeries(page.readings));
        setTruncated(page.truncated);
      } catch (cause) {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    })();
    return () => controller.abort();
  }, [plant.id, device.id, range]);

  const options = availableKeys.map((key) => {
    const meta = pointMeta(catalog, key);
    return { label: meta.displayName, value: key, unit: meta.unit, tag: meta.tag };
  });

  const series = useMemo<ChartSeries[]>(
    () => selectedKeys.map((key, index) => {
      const meta = pointMeta(catalog, key);
      return {
        key,
        label: meta.displayName,
        unit: meta.unit,
        decimals: meta.decimals,
        color: SERIES_COLORS[index % SERIES_COLORS.length],
        points: seriesByKey[key] ?? [],
      };
    }),
    [selectedKeys, catalog, seriesByKey],
  );

  return (
    <section className="device-chart-panel">
      <div className="section-heading">
        <div><p>Telemetry history</p><h3>กราฟค่าย้อนหลัง</h3></div>
        <div className="flex flex-wrap items-center gap-2">
          <TextInput
            type="datetime-local"
            className="h-10 rounded-[var(--radius-sm)] border border-line px-2 text-sm"
            value={toDatetimeLocal(range.from)}
            onChange={(event) => {
              const from = new Date(event.target.value);
              if (!Number.isNaN(+from)) setRange((current) => ({ ...current, from }));
            }}
          />
          <span className="text-xs text-slate-500">ถึง</span>
          <TextInput
            type="datetime-local"
            className="h-10 rounded-[var(--radius-sm)] border border-line px-2 text-sm"
            value={toDatetimeLocal(range.to)}
            onChange={(event) => {
              const to = new Date(event.target.value);
              if (!Number.isNaN(+to)) setRange((current) => ({ ...current, to }));
            }}
          />
        </div>
      </div>
      <div className="px-1 pb-3">
        <MultiSelect
          value={selectedKeys}
          onValueChange={setSelectedKeys}
          options={options}
          placeholder="เลือก Parameter เพื่อ plot กราฟ"
          ariaLabel="เลือก Parameter เพื่อ plot กราฟ"
          disabled={availableKeys.length === 0}
        />
      </div>
      {error && <FormMessage>{error}</FormMessage>}
      {truncated && <p className="ts-truncated">ข้อมูลในช่วงนี้มากเกินไป แสดงเฉพาะส่วนล่าสุด — ลากบนกราฟเพื่อ zoom เข้าไปดูช่วงที่ต้องการ</p>}
      {!error && selectedKeys.length === 0 && <div className="table-state">เลือก Parameter อย่างน้อยหนึ่งตัวเพื่อดูกราฟ</div>}
      {!error && selectedKeys.length > 0 && (
        <TimeSeriesChart
          series={series}
          from={range.from.getTime()}
          to={range.to.getTime()}
          onZoom={(from, to) => setRange({ from, to })}
          onResetZoom={() => setRange(defaultRange())}
        />
      )}
      {loading && <p className="ts-hint">กำลังโหลดข้อมูล…</p>}
    </section>
  );
}
