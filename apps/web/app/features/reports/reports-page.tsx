"use client";

import { Download, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { api, downloadBlob, errorMessage, toDatetimeLocal } from "../../lib/api";
import { fetchRange, loadRegisterCatalogs } from "../../lib/telemetry-history";
import { calculatedReportCSV, type CalculatedReportRow } from "../../lib/calculated-report-csv";
import { toSeries, totalEnergyKWh } from "../../lib/telemetry-math";
import type { Device, Plant } from "../../lib/types";
import { Button } from "../../components/ui/button";
import { FormMessage, TextInput } from "../../components/ui/form";
import { MultiSelect } from "../../components/ui/multi-select";

export function ReportsPage() {
  const now = new Date();
  const [plants, setPlants] = useState<Plant[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [plantIds, setPlantIds] = useState<string[]>([]);
  const [deviceIds, setDeviceIds] = useState<string[]>([]);
  const [from, setFrom] = useState(toDatetimeLocal(new Date(now.getTime() - 24 * 60 * 60 * 1000)));
  const [to, setTo] = useState(toDatetimeLocal(now));
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    void api("/api/v1/plants").then(async (response) => {
      if (!response.ok) throw new Error("ไม่สามารถโหลดรายชื่อโรงไฟฟ้าได้");
      setPlants((await response.json()) as Plant[]);
    }).catch((cause) => setError(errorMessage(cause)));
  }, []);

  useEffect(() => {
    setDeviceIds([]);
    if (plantIds.length === 0) { setDevices([]); return; }
    void Promise.all(plantIds.map(async (plantId) => {
      const response = await api(`/api/v1/plants/${encodeURIComponent(plantId)}/devices`);
      if (!response.ok) throw new Error("ไม่สามารถโหลด Device ได้");
      return (await response.json()) as Device[];
    })).then((groups) => setDevices(groups.flat())).catch((cause) => setError(errorMessage(cause)));
  }, [plantIds]);

  async function exportReport() {
    if (deviceIds.length === 0) return;
    const start = new Date(from), end = new Date(to);
    if (!Number.isFinite(+start) || !Number.isFinite(+end) || end <= start) { setError("ช่วงเวลาไม่ถูกต้อง"); return; }
    setLoading(true); setError("");
    try {
      const selected = devices.filter((device) => deviceIds.includes(device.id));
      const catalogs = await Promise.all(plantIds.map(async (plantId) => [plantId, await loadRegisterCatalogs(plantId, selected.filter((device) => device.plantId === plantId))] as const));
      const catalogByPlant = Object.fromEntries(catalogs);
      const output: CalculatedReportRow[] = [];
      for (const device of selected) {
        const plant = plants.find((item) => item.id === device.plantId);
        if (!plant) continue;
        const page = await fetchRange(plant.id, device.id, start, end);
        const series = toSeries(page.readings);
        const catalog = catalogByPlant[plant.id]?.[device.id] ?? {};
        const energyKey = Object.keys(series).find((key) => /power|energy|kw|w/i.test(catalog[key]?.unit ?? ""));
        const kWh = energyKey ? totalEnergyKWh(series[energyKey], catalog[energyKey]?.unit ?? "") : null;
        output.push(...page.readings.map((reading) => ({ plant: plant.name, device: device.name, observedAt: reading.observedAt, values: reading.dataItemMap, kWh })));
      }
      const csv = calculatedReportCSV(output);
      downloadBlob(new Blob([`﻿${csv}`], { type: "text/csv;charset=utf-8" }), `ygate-calculated-report-${new Date().toISOString().slice(0, 10)}.csv`);
    } catch (cause) { setError(errorMessage(cause)); }
    finally { setLoading(false); }
  }

  return <div className="content reports-content">
    <div className="section-heading"><div><p>Calculated telemetry</p><h2>Reports</h2></div><div className="heading-actions"><Button onClick={() => void exportReport()} disabled={loading || deviceIds.length === 0}>{loading ? <RefreshCw className="spin" size={16} /> : <Download size={16} />}{loading ? "กำลังสร้าง CSV..." : "ดาวน์โหลด CSV"}</Button></div></div>
    {error && <FormMessage>{error}</FormMessage>}
    <section className="ea-panel report-form"><div className="ea-panel-head"><h3>Export calculated records</h3><span className="ts-unit">ทุก parameter อยู่ใน CSV เดียว</span></div>
      <label className="ea-field">Plant<MultiSelect value={plantIds} onValueChange={setPlantIds} options={plants.map((plant) => ({ label: `${plant.code} — ${plant.name}`, value: plant.id }))} placeholder="เลือก Plant" ariaLabel="เลือก Plant" /></label>
      <label className="ea-field">Device<MultiSelect value={deviceIds} onValueChange={setDeviceIds} options={devices.map((device) => ({ label: device.name, value: device.id, tag: device.externalId }))} placeholder="เลือก Device" ariaLabel="เลือก Device" disabled={devices.length === 0} /></label>
      <div className="report-date-grid"><label className="ea-field">เริ่มต้น<TextInput className="ea-input" type="datetime-local" value={from} onChange={(event) => setFrom(event.target.value)} /></label><label className="ea-field">สิ้นสุด<TextInput className="ea-input" type="datetime-local" value={to} onChange={(event) => setTo(event.target.value)} /></label></div>
    </section>
  </div>;
}
