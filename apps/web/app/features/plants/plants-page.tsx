"use client";

import { ArchiveX, ArrowLeft, Cpu, Download, Eye, MapPin, Pencil, PlugZap, Plus, RefreshCw, Search, Trash2, Upload, Wifi } from "lucide-react";
import { Checkbox, FormMessage, StatusTag, TextInput } from "../../components/ui/form";
import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { api, errorMessage, assetURL, csrfToken, downloadBlob, formatDate } from "../../lib/api";
import { useRealtimeSocket } from "../../lib/realtime";
import type { Device, DeviceModelOption, LatestTelemetry, Plant } from "../../lib/types";
import { loadRegisterCatalog, type PointMeta } from "../../lib/telemetry-history";
import { DeviceHistoryChart } from "./device-history-chart";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";
import { DataTable, TableColumn } from "../../components/ui/data-table";

export function PlantsPage({ defaultOrganizationId }: { defaultOrganizationId?: string }) {
  const [plants, setPlants] = useState<Plant[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<Plant | "create" | null>(null);
  const [selectedPlant, setSelectedPlant] = useState<Plant | null>(null);
  const [query, setQuery] = useState("");

  const loadPlants = useCallback(async () => {
    setLoading(true);
    setError("");
    setPlants([]);
    try {
      const response = await api("/api/v1/plants");
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดูข้อมูลโรงไฟฟ้า");
      if (!response.ok) throw new Error("ไม่สามารถโหลดข้อมูลโรงไฟฟ้าได้");
      setPlants((await response.json()) as Plant[]);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadPlants(); }, [loadPlants]);

  // Deep-link from Site Map ("ดูรายละเอียดโรงไฟฟ้า" -> /plants?open=<id>) straight
  // into that plant's device management view once the list has loaded.
  useEffect(() => {
    if (plants.length === 0) return;
    const openId = new URLSearchParams(window.location.search).get("open");
    if (!openId) return;
    const match = plants.find((plant) => plant.id === openId);
    if (match) setSelectedPlant(match);
    window.history.replaceState(null, "", window.location.pathname);
  }, [plants]);

  async function decommissionPlant(plant: Plant) {
    if (!window.confirm(`ปิดใช้งานโรงไฟฟ้า “${plant.name}”? ข้อมูลเดิมจะยังอยู่ แต่ Middleware จะส่งข้อมูลเข้า Plant นี้ไม่ได้`)) return;
    const response = await api(`/api/v1/plants/${plant.id}`, {
      method: "PUT",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ code: plant.code, name: plant.name, timezone: plant.timezone, latitude: plant.latitude, longitude: plant.longitude, installedDcKw: plant.installedDcKw, installedAcKw: plant.installedAcKw, isActive: false }),
    });
    if (response.ok) { toast.success(`ปิดใช้งานโรงไฟฟ้า "${plant.name}" แล้ว`); void loadPlants(); }
    else setError("ไม่สามารถปิดใช้งานโรงไฟฟ้าได้");
  }

  async function hardDeletePlant(plant: Plant) {
    const expected = "DELETE";
    if (window.prompt(`คำสั่งนี้จะลบ Plant, Device, Metadata และ normalized telemetry ถาวร\nพิมพ์ ${expected}`) !== expected) return;
    const response = await api(`/api/v1/plants/${encodeURIComponent(plant.id)}`, {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrfToken(), "X-Hard-Delete-Confirm": expected },
    });
    if (response.ok) { toast.success(`ลบโรงไฟฟ้า "${plant.name}" ถาวรแล้ว`); await loadPlants(); }
    else setError(response.status === 403 ? "เฉพาะ Platform Admin เท่านั้นที่ลบ Plant ถาวรได้" : "ไม่สามารถลบ Plant ถาวรได้");
  }

  const importInputRef = useRef<HTMLInputElement>(null);

  async function exportAllCSV() {
    const response = await api("/api/v1/plants/export-all");
    if (!response.ok) { toast.error("ดาวน์โหลด CSV ไม่สำเร็จ"); return; }
    downloadBlob(await response.blob(), "plants-devices.csv");
  }

  async function exportOneCSV(plant: Plant) {
    const response = await api(`/api/v1/plants/${encodeURIComponent(plant.id)}/export`);
    if (!response.ok) { toast.error("ดาวน์โหลด CSV ไม่สำเร็จ"); return; }
    downloadBlob(await response.blob(), `${plant.code}-devices.csv`);
  }

  async function downloadPlantTemplate() {
    const response = await api("/api/v1/plants/import-template");
    if (!response.ok) { toast.error("ดาวน์โหลด Template ไม่สำเร็จ"); return; }
    downloadBlob(await response.blob(), "plant-device-template.csv");
  }

  async function importPlantCSVFile(file: File) {
    const formData = new FormData();
    formData.append("file", file);
    try {
      const response = await api("/api/v1/plants/import", { method: "POST", headers: { "X-CSRF-Token": csrfToken() }, body: formData });
      if (!response.ok) throw new Error("Import CSV ไม่สำเร็จ — ตรวจรูปแบบไฟล์");
      const result = (await response.json()) as { plantsCreated: number; plantsUpdated: number; devicesCreated: number; devicesUpdated: number; rowsSkipped: number; errors?: string[] };
      toast.success(
        `Import สำเร็จ: Plant ใหม่ ${result.plantsCreated}, Plant อัปเดต ${result.plantsUpdated}, Device ใหม่ ${result.devicesCreated}, Device อัปเดต ${result.devicesUpdated}`
        + (result.rowsSkipped > 0 ? `, ข้าม ${result.rowsSkipped} แถว (${(result.errors ?? []).slice(0, 2).join("; ")})` : ""),
      );
      await loadPlants();
    } catch (cause) {
      toast.error(errorMessage(cause));
    }
  }

  if (selectedPlant) {
    return <DeviceManagement plant={selectedPlant} onBack={() => setSelectedPlant(null)} />;
  }

  const normalizedQuery = query.trim().toLowerCase();
  const visiblePlants = normalizedQuery
    ? plants.filter((plant) =>
      plant.name.toLowerCase().includes(normalizedQuery)
      || plant.code.toLowerCase().includes(normalizedQuery)
      || plant.organizationName.toLowerCase().includes(normalizedQuery))
    : plants;

  return (
    <div className="content plants-content">
      <div className="section-heading">
        <div><p>Plant registry</p><h2>โรงไฟฟ้าทั้งหมด</h2></div>
        <div className="heading-actions">
          <Button variant="icon" onClick={() => void loadPlants()} title="รีเฟรช" aria-label="รีเฟรชรายการโรงไฟฟ้า"><RefreshCw size={18} /></Button>
          <Button variant="secondary" compact onClick={() => void exportAllCSV()} title="Export CSV (ทุกโรงไฟฟ้า)"><Download size={16} /> Export CSV</Button>
          <Button variant="secondary" compact onClick={() => void downloadPlantTemplate()} title="ดาวน์โหลด Template เปล่า">Template</Button>
          <Button variant="secondary" compact onClick={() => importInputRef.current?.click()} title="Import CSV (อัปเดตทับของเดิม)"><Upload size={16} /> Import CSV</Button>
          <input
            ref={importInputRef}
            type="file"
            accept=".csv,text/csv"
            className="hidden"
            onChange={(event) => {
              const file = event.target.files?.[0];
              if (file) void importPlantCSVFile(file);
              event.target.value = "";
            }}
          />
          <Button compact onClick={() => setEditor("create")}><Plus size={18} /> เพิ่มโรงไฟฟ้า</Button>
        </div>
      </div>
      <div className="plant-search">
        <Search size={16} />
        <TextInput
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="ค้นหาโรงไฟฟ้า ด้วยชื่อ, รหัส หรือ องค์กร"
          aria-label="ค้นหาโรงไฟฟ้า"
        />
      </div>
      {error && <FormMessage>{error}</FormMessage>}
      {loading ? (
        <div className="table-state">กำลังโหลดข้อมูล</div>
      ) : (
        <DataTable
          value={visiblePlants}
          dataKey="id"
          aria-label="โรงไฟฟ้า"
          paginator
          rows={20}
          emptyMessage={
            <div className="table-state">
              {error ? "" : plants.length === 0 ? "ยังไม่มีโรงไฟฟ้าในขอบเขตที่คุณเข้าถึงได้" : `ไม่พบโรงไฟฟ้าที่ตรงกับ "${query}"`}
            </div>
          }
        >
          <TableColumn field="name" header="โรงไฟฟ้า" sortable body={(plant: Plant) => (
            <div className="grid gap-1"><strong>{plant.name}</strong><small className="flex items-center gap-1 text-[11px] text-ink-soft"><MapPin size={13} /> {plant.code} · {plant.timezone}</small></div>
          )} />
          <TableColumn field="organizationName" header="องค์กร" sortable body={(plant: Plant) => (
            <div className="grid gap-1"><span>{plant.organizationName}</span><small className="block text-[11px] text-ink-soft">{plant.organizationId}</small></div>
          )} />
          <TableColumn field="installedDcKw" header="กำลังติดตั้ง" sortable body={(plant: Plant) => (
            <div className="grid gap-1"><span>{plant.installedDcKw == null ? "-" : `${plant.installedDcKw.toLocaleString()} kWdc`}</span><small className="block text-[11px] text-ink-soft">{plant.installedAcKw == null ? "ไม่ระบุ AC" : `${plant.installedAcKw.toLocaleString()} kWac`}</small></div>
          )} />
          <TableColumn field="isActive" header="สถานะ" sortable body={(plant: Plant) => (
            <StatusTag tone={plant.isActive ? "active" : "revoked"}>{plant.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</StatusTag>
          )} />
          <TableColumn header="" body={(plant: Plant) => (
            <div className="row-actions">
              <Button variant="icon" onClick={() => setSelectedPlant(plant)} title="จัดการ Device" aria-label={`จัดการ Device ใน ${plant.name}`}><Cpu size={17} /></Button>
              <Button variant="icon" onClick={() => void exportOneCSV(plant)} title="Export CSV" aria-label={`Export CSV ของ ${plant.name}`}><Download size={17} /></Button>
              <Button variant="icon" onClick={() => setEditor(plant)} title="แก้ไขโรงไฟฟ้า" aria-label={`แก้ไข ${plant.name}`}><Pencil size={17} /></Button>
              {plant.isActive && <Button variant="icon" onClick={() => void decommissionPlant(plant)} title="ปิดใช้งาน" aria-label={`ปิดใช้งาน ${plant.name}`}><ArchiveX size={17} /></Button>}
              <Button variant="icon" danger onClick={() => void hardDeletePlant(plant)} title="ลบถาวร (Platform Admin)" aria-label={`ลบ ${plant.name} ถาวร`}><Trash2 size={17} /></Button>
            </div>
          )} />
        </DataTable>
      )}
      {editor && <PlantEditor plant={editor === "create" ? undefined : editor} defaultOrganizationId={defaultOrganizationId} onClose={() => setEditor(null)} onSaved={() => { setEditor(null); void loadPlants(); }} />}
    </div>
  );
}

function DeviceManagement({ plant, onBack }: { plant: Plant; onBack: () => void }) {
  const [devices, setDevices] = useState<Device[]>([]);
  const [latestByDevice, setLatestByDevice] = useState<Record<string, LatestTelemetry>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<Device | "create" | null>(null);
  const [selectedDevice, setSelectedDevice] = useState<Device | null>(null);
  const [testOutcomes, setTestOutcomes] = useState<Record<string, { pending: boolean }>>({});
  const [testReadResult, setTestReadResult] = useState<{ device: Device; dataItemMap: Record<string, number>; collectTime?: number } | null>(null);

  const loadDevices = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [deviceResponse, telemetryResponse] = await Promise.all([
        api(`/api/v1/plants/${plant.id}/devices`),
        api(`/api/v1/plants/${plant.id}/telemetry/latest`),
      ]);
      if (deviceResponse.status === 404 || telemetryResponse.status === 404) throw new Error("ไม่พบโรงไฟฟ้าหรือบัญชีนี้ไม่มีสิทธิ์เข้าถึง Device");
      if (!deviceResponse.ok || !telemetryResponse.ok) throw new Error("ไม่สามารถโหลดข้อมูล Device ได้");
      setDevices((await deviceResponse.json()) as Device[]);
      const readings = (await telemetryResponse.json()) as LatestTelemetry[];
      setLatestByDevice(Object.fromEntries(readings.map((reading) => [reading.deviceId, reading])));
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, [plant.id]);

  useEffect(() => { void loadDevices(); }, [loadDevices]);

  const liveState = useRealtimeSocket(plant.id, (message) => {
    if (message.type === "telemetry.snapshot") {
      setLatestByDevice(Object.fromEntries(message.data.map((reading) => [reading.deviceId, reading])));
    }
  });

  async function decommissionDevice(device: Device) {
    if (!window.confirm(`ปิดใช้งาน Device “${device.name}”? ข้อมูล telemetry เดิมจะยังคงอยู่`)) return;
    const response = await api(`/api/v1/plants/${plant.id}/devices/${device.id}`, {
      method: "PUT",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({
        name: device.name, deviceModelId: device.deviceModelId, modbusHost: device.modbusHost ?? "",
        modbusPort: device.modbusPort ?? null, modbusUnitId: device.modbusUnitId,
        isActive: false,
      }),
    });
    if (response.ok) { toast.success(`ปิดใช้งาน Device "${device.name}" แล้ว`); void loadDevices(); }
    else setError("ไม่สามารถปิดใช้งาน Device ได้");
  }

  async function hardDeleteDevice(device: Device) {
    const expected = "DELETE";
    if (window.prompt(`คำสั่งนี้จะลบ Device, Metadata และ normalized telemetry ถาวร\nพิมพ์ ${expected}`) !== expected) return;
    const response = await api(`/api/v1/plants/${encodeURIComponent(plant.id)}/devices/${encodeURIComponent(device.id)}`, {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrfToken(), "X-Hard-Delete-Confirm": expected },
    });
    if (response.ok) { toast.success(`ลบ Device "${device.name}" ถาวรแล้ว`); await loadDevices(); }
    else setError(response.status === 403 ? "เฉพาะ Platform Admin เท่านั้นที่ลบ Device ถาวรได้" : "ไม่สามารถลบ Device ถาวรได้");
  }

  async function runCommand(kind: "test-connection" | "test-read", device: Device) {
    setTestOutcomes((prev) => ({ ...prev, [device.id]: { pending: true } }));
    try {
      const response = await api(`/api/v1/plants/${plant.id}/devices/${device.id}/${kind}`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
      });
      if (response.status === 503) throw new Error("ไม่มี Middleware ดูแล Plant นี้อยู่ หรือออฟไลน์อยู่");
      if (response.status === 504) throw new Error("Middleware ไม่ตอบสนองภายในเวลาที่กำหนด");
      if (!response.ok) throw new Error("ทดสอบไม่สำเร็จ");
      const data = (await response.json()) as {
        ok?: boolean;
        error?: string;
        data?: { reading?: { registerAddressMap?: Record<string, number>; collectTime?: number } };
      };
      setTestOutcomes((prev) => ({ ...prev, [device.id]: { pending: false } }));
      if (data.ok === false) {
        toast.error(data.error || "ทดสอบไม่สำเร็จ");
        return;
      }
      if (kind === "test-connection") {
        toast.success("เชื่อมต่อสำเร็จ");
        return;
      }
      // Show the actual register values read, not just a success toast --
      // "success" alone doesn't tell the user whether the values look right.
      const dataItemMap = data.data?.reading?.registerAddressMap ?? {};
      const count = Object.keys(dataItemMap).length;
      setTestReadResult({ device, dataItemMap, collectTime: data.data?.reading?.collectTime });
      toast.success(count > 0 ? `อ่านค่าสำเร็จ (${count} ค่า)` : "อ่านค่าสำเร็จ แต่ไม่มีค่า Register กลับมา — เช็ก Register Metadata ของ Device Model นี้");
    } catch (cause) {
      const message = errorMessage(cause);
      setTestOutcomes((prev) => ({ ...prev, [device.id]: { pending: false } }));
      toast.error(message);
    }
  }

  const latestReadings = Object.values(latestByDevice);
  const lastObservedAt = latestReadings.reduce<string | undefined>((latest, reading) => !latest || reading.observedAt > latest ? reading.observedAt : latest, undefined);

  if (selectedDevice) {
    return (
      <DeviceDetailView
        plant={plant}
        device={selectedDevice}
        reading={latestByDevice[selectedDevice.id]}
        onBack={() => setSelectedDevice(null)}
      />
    );
  }

  return (
    <div className="content devices-content">
      <div className="section-heading">
        <div className="registry-title">
          <Button variant="icon" onClick={onBack} title="กลับไปโรงไฟฟ้า" aria-label="กลับไปโรงไฟฟ้า"><ArrowLeft size={18} /></Button>
          <div><p>{plant.code}</p><h2>Device ใน {plant.name}</h2></div>
        </div>
        <div className="row-actions">
          <span className={`live-chip ${liveState}`}><Wifi size={15} />{liveState === "connected" ? "Live data" : liveState === "connecting" ? "Connecting" : "Offline"}</span>
          <Button variant="icon" onClick={() => void loadDevices()} title="รีเฟรช" aria-label="รีเฟรชรายการ Device"><RefreshCw size={18} /></Button>
          <Button compact onClick={() => setEditor("create")}><Plus size={18} /> เพิ่ม Device</Button>
        </div>
      </div>
      {error && <FormMessage>{error}</FormMessage>}
      <section className="grid gap-px overflow-hidden rounded-md border border-slate-200 bg-slate-200 sm:grid-cols-2 xl:grid-cols-4" aria-label="Plant summary">
        <div className="bg-white p-4"><small className="font-bold text-slate-500">Installed capacity</small><strong className="mt-1 block text-xl text-slate-900">{plant.installedDcKw?.toLocaleString() ?? "-"} kWdc</strong><span className="text-xs text-slate-500">{plant.installedAcKw?.toLocaleString() ?? "-"} kWac</span></div>
        <div className="bg-white p-4"><small className="font-bold text-slate-500">Devices</small><strong className="mt-1 block text-xl text-slate-900">{devices.length.toLocaleString()}</strong><span className="text-xs text-slate-500">ใช้งาน {devices.filter((device) => device.isActive).length.toLocaleString()}</span></div>
        <div className="bg-white p-4"><small className="font-bold text-slate-500">Reporting</small><strong className="mt-1 block text-xl text-slate-900">{latestReadings.length.toLocaleString()}</strong><span className="text-xs text-slate-500">Device ที่มีค่าล่าสุด</span></div>
        <div className="bg-white p-4"><small className="font-bold text-slate-500">Latest observed</small><strong className="mt-1 block text-sm text-slate-900">{lastObservedAt ? formatDate(lastObservedAt) : "ยังไม่มีข้อมูล"}</strong><span className="text-xs text-slate-500">Timezone: {plant.timezone}</span></div>
      </section>
      {loading ? (
        <div className="table-state">กำลังโหลดข้อมูล</div>
      ) : (
        <DataTable
          value={devices}
          dataKey="id"
          aria-label={`Device ใน ${plant.name}`}
          paginator
          rows={20}
          emptyMessage={<div className="table-state">{error ? "" : "ยังไม่มี Device กดเพิ่ม Device หรือให้ Middleware auto onboard เมื่อส่งข้อมูลเข้ามา"}</div>}
        >
          <TableColumn field="name" header="Device" sortable body={(device: Device) => (
            <div className="grid gap-1"><strong>{device.name}</strong><small className="block text-[11px] text-ink-soft">{device.externalId}</small></div>
          )} />
          <TableColumn field="model" header="รุ่น" sortable body={(device: Device) => (
            <div className="grid gap-1"><span>{device.model}</span><small className="block text-[11px] text-ink-soft">{device.manufacturer}</small></div>
          )} />
          <TableColumn header="ประเภท" body={(device: Device) => (
            <div className="grid gap-1"><span>{device.modbusHost ? `${device.modbusHost}:${device.modbusPort}` : "ไม่ใช่ Modbus device"}</span><small className="block text-[11px] text-ink-soft">{device.modbusHost ? `unit ${device.modbusUnitId}` : device.deviceType}</small></div>
          )} />
          <TableColumn header="ค่าล่าสุด" body={(device: Device) => <LatestValues reading={latestByDevice[device.id]} />} />
          <TableColumn field="isActive" header="สถานะ" sortable body={(device: Device) => (
            <StatusTag tone={device.isActive ? "active" : "revoked"}>{device.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</StatusTag>
          )} />
          <TableColumn header="" body={(device: Device) => {
            const outcome = testOutcomes[device.id];
            const canTest = Boolean(device.modbusHost && device.modbusPort);
            return (
              <div className="row-actions">
                <Button variant="icon" disabled={!canTest || outcome?.pending} onClick={() => void runCommand("test-connection", device)} title={canTest ? "ทดสอบการเชื่อมต่อ" : "ต้องตั้งค่า IP/Port ก่อน"} aria-label={`ทดสอบการเชื่อมต่อ ${device.name}`}><PlugZap size={17} /></Button>
                <Button variant="icon" disabled={!canTest || outcome?.pending} onClick={() => void runCommand("test-read", device)} title="ทดสอบอ่านค่า" aria-label={`ทดสอบอ่านค่า ${device.name}`}><RefreshCw size={17} /></Button>
                <Button variant="icon" onClick={() => setSelectedDevice(device)} title="ดูค่าล่าสุดทั้งหมด" aria-label={`ดูค่าล่าสุดของ ${device.name}`}><Eye size={17} /></Button>
                <Button variant="icon" onClick={() => setEditor(device)} title="แก้ไข Device" aria-label={`แก้ไข ${device.name}`}><Pencil size={17} /></Button>
                {device.isActive && <Button variant="icon" onClick={() => void decommissionDevice(device)} title="ปิดใช้งาน" aria-label={`ปิดใช้งาน ${device.name}`}><ArchiveX size={17} /></Button>}
                <Button variant="icon" danger onClick={() => void hardDeleteDevice(device)} title="ลบถาวร (Platform Admin)" aria-label={`ลบ ${device.name} ถาวร`}><Trash2 size={17} /></Button>
              </div>
            );
          }} />
        </DataTable>
      )}
      {testReadResult && <TestReadDialog result={testReadResult} onClose={() => setTestReadResult(null)} />}
      {editor && <DeviceEditor plant={plant} device={editor === "create" ? undefined : editor} onClose={() => setEditor(null)} onSaved={() => { setEditor(null); void loadDevices(); }} />}
    </div>
  );
}

function DeviceDetailView({ plant, device, reading, onBack }: { plant: Plant; device: Device; reading?: LatestTelemetry; onBack: () => void }) {
  const [metadataByKey, setMetadataByKey] = useState<Record<string, PointMeta>>({});
  const [metadataError, setMetadataError] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    void loadRegisterCatalog(plant.id, device, controller.signal)
      .then(setMetadataByKey)
      .catch(() => {
        if (!controller.signal.aborted) setMetadataError("ไม่สามารถโหลด Unit ของ Parameter ได้");
      });
    return () => controller.abort();
  }, [plant.id, device]);

  const values = Object.entries(reading?.dataItemMap ?? {})
    .filter(([key]) => metadataByKey[key]?.isEnabled !== false)
    .sort(([a], [b]) => a.localeCompare(b));
  const availableKeys = values.map(([key]) => key);

  return (
    <div className="content device-detail-content">
      <div className="section-heading">
        <div className="registry-title">
          <Button variant="icon" onClick={onBack} title="กลับไป Device" aria-label="กลับไป Device"><ArrowLeft size={18} /></Button>
          <div><p>{plant.code} · {device.externalId}{reading ? ` · ${formatDate(reading.observedAt)}` : ""}</p><h2>{device.name}</h2></div>
        </div>
      </div>
      {metadataError && <FormMessage>{metadataError}</FormMessage>}
      <section className="grid gap-px overflow-hidden rounded-md border border-slate-200 bg-slate-200 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6" aria-label="ค่าปัจจุบัน (เฉพาะ parameter ที่เปิดใช้งาน)">
        {values.length === 0 && <div className="table-state col-span-full">ยังไม่มี telemetry (เฉพาะ parameter ที่เปิดใช้งาน) สำหรับ Device นี้</div>}
        {values.map(([key, value]) => {
          const meta = metadataByKey[key];
          return (
            <div className="bg-white p-2.5" key={key}>
              <small className="block truncate text-[11px] font-bold text-slate-500" title={meta?.displayName || key}>{meta?.displayName || key}</small>
              <strong className="mt-0.5 block truncate text-base text-slate-900" title={String(value)}>
                {Number.isFinite(value) ? value.toLocaleString(undefined, meta ? { minimumFractionDigits: meta.decimals, maximumFractionDigits: meta.decimals } : undefined) : "-"}
                {meta?.unit ? <span className="ml-1 text-xs font-normal text-slate-500">{meta.unit}</span> : null}
              </strong>
            </div>
          );
        })}
      </section>
      <DeviceHistoryChart plant={plant} device={device} catalog={metadataByKey} availableKeys={availableKeys} />
    </div>
  );
}

function TestReadDialog({ result, onClose }: { result: { device: Device; dataItemMap: Record<string, number>; collectTime?: number }; onClose: () => void }) {
  const entries = Object.entries(result.dataItemMap).sort(([a], [b]) => a.localeCompare(b));
  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div><DialogDescription>{result.device.externalId}</DialogDescription><DialogTitle>ผลทดสอบอ่านค่า {result.device.name}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          {result.collectTime && <div className="px-5 pt-4 text-sm"><small className="block text-slate-500">อ่านเมื่อ</small><strong>{formatDate(new Date(result.collectTime).toISOString())}</strong></div>}
          <div className="max-h-[55vh] overflow-auto border-t border-slate-200 px-5 py-3">
            {entries.length === 0
              ? <div className="table-state">ไม่มีค่า Register กลับมา</div>
              : entries.map(([key, value]) => (
                <div key={key} className="flex items-center justify-between gap-4 border-b border-slate-100 py-2 text-sm last:border-0">
                  <code className="text-slate-600">{key}</code><strong className="text-slate-900">{Number.isFinite(value) ? value.toLocaleString() : "-"}</strong>
                </div>
              ))}
          </div>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

function LatestValues({ reading }: { reading?: LatestTelemetry }) {
  if (!reading) return <div><span>-</span><small>ยังไม่มี telemetry</small></div>;
  const values = Object.entries(reading.dataItemMap).slice(0, 3);
  return (
    <div className="grid gap-0.5" title={`รับเมื่อ ${formatDate(reading.receivedAt)}`}>
      {values.map(([key, value]) => <small key={key}>{key}: {Number.isFinite(value) ? value.toLocaleString() : "-"}</small>)}
      {reading.parameterCount > values.length && <small>+{reading.parameterCount - values.length} parameters</small>}
    </div>
  );
}

function DeviceEditor({ plant, device, onClose, onSaved }: { plant: Plant; device?: Device; onClose: () => void; onSaved: () => void }) {
  const [models, setModels] = useState<DeviceModelOption[]>([]);
  const [externalId, setExternalId] = useState(device?.externalId ?? "");
  const [name, setName] = useState(device?.name ?? "");
  const [deviceModelId, setDeviceModelId] = useState(device?.deviceModelId ?? "");
  const [modbusHost, setModbusHost] = useState(device?.modbusHost ?? "");
  const [modbusPort, setModbusPort] = useState(device?.modbusPort?.toString() ?? "502");
  const [modbusUnitId, setModbusUnitId] = useState(device?.modbusUnitId?.toString() ?? "1");
  const [isActive, setIsActive] = useState(device?.isActive ?? true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    void (async () => {
      const response = await api("/api/v1/device-models");
      if (response.ok) {
        const list = (await response.json()) as DeviceModelOption[];
        setModels(list);
        if (!deviceModelId && list.length > 0) setDeviceModelId(list[0].id);
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    const host = modbusHost.trim();
    const body = {
      ...(device ? {} : { externalId }),
      name, deviceModelId,
      modbusHost: host,
      modbusPort: host === "" ? null : Number(modbusPort),
      modbusUnitId: Number(modbusUnitId),
      isActive,
    };
    try {
      const response = await api(device ? `/api/v1/plants/${plant.id}/devices/${device.id}` : `/api/v1/plants/${plant.id}/devices`, {
        method: device ? "PUT" : "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify(body),
      });
      if (response.status === 404) throw new Error("ไม่พบ Plant/Device หรือบัญชีนี้ไม่มีสิทธิ์");
      if (response.status === 409) throw new Error("External ID นี้มีอยู่แล้วใน Plant");
      if (!response.ok) {
        const detail = await response.text().catch(() => "");
        throw new Error(`บันทึกไม่ได้ (${response.status})${detail ? `: ${detail}` : ""}`);
      }
      onSaved();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !pending) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div><DialogDescription>{plant.code}</DialogDescription><DialogTitle>{device ? "แก้ไข Device" : "เพิ่ม Device"}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={submit}>
            <label>Device ID<TextInput autoFocus={!device} value={externalId} onChange={(event) => setExternalId(event.target.value)} maxLength={200} required disabled={Boolean(device)} /></label>
            <label>ชื่อ Device<TextInput autoFocus={Boolean(device)} value={name} onChange={(event) => setName(event.target.value)} maxLength={200} required /></label>
            <label className="full-field">Model
              <Select value={deviceModelId} onValueChange={setDeviceModelId} disabled={models.length === 0}>
                <SelectTrigger><SelectValue placeholder="ยังไม่มี Device Model — สร้างที่หน้า Register Metadata ก่อน" /></SelectTrigger>
                <SelectContent>{models.map((m) => <SelectItem key={m.id} value={m.id}>{m.manufacturer} / {m.deviceType} / {m.model}</SelectItem>)}</SelectContent>
              </Select>
            </label>
            <label>IP<TextInput value={modbusHost} onChange={(event) => setModbusHost(event.target.value)} placeholder="192.168.1.100 (เว้นว่างถ้าไม่ใช่ Modbus device)" /></label>
            <label>Port<TextInput type="number" min="1" max="65535" value={modbusPort} onChange={(event) => setModbusPort(event.target.value)} /></label>
            <label>Unit ID<TextInput type="number" min="0" max="255" value={modbusUnitId} onChange={(event) => setModbusUnitId(event.target.value)} /></label>
            <label className="toggle-field full-field"><Checkbox checked={isActive} onChange={setIsActive} /><span>เปิดใช้งาน Device</span></label>
            {error && <FormMessage className="full-field">{error}</FormMessage>}
            <div className="editor-actions full-field"><Button type="button" variant="secondary" onClick={onClose} disabled={pending}>ยกเลิก</Button><Button disabled={pending}>{pending ? "กำลังบันทึก" : device ? "บันทึก" : "สร้าง Device"}</Button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

function PlantEditor({ plant, defaultOrganizationId, onClose, onSaved }: { plant?: Plant; defaultOrganizationId?: string; onClose: () => void; onSaved: () => void }) {
  const [organizationId, setOrganizationId] = useState(plant?.organizationId ?? defaultOrganizationId ?? "");
  const [code, setCode] = useState(plant?.code ?? "");
  const [name, setName] = useState(plant?.name ?? "");
  const [timezone, setTimezone] = useState(plant?.timezone ?? "Asia/Bangkok");
  const [latitude, setLatitude] = useState(plant?.latitude?.toString() ?? "");
  const [longitude, setLongitude] = useState(plant?.longitude?.toString() ?? "");
  const [installedDcKw, setInstalledDcKw] = useState(plant?.installedDcKw?.toString() ?? "");
  const [installedAcKw, setInstalledAcKw] = useState(plant?.installedAcKw?.toString() ?? "");
  const [isActive, setIsActive] = useState(plant?.isActive ?? true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [imagePreview, setImagePreview] = useState(plant?.imageUrl ? assetURL(plant.imageUrl) : null);
  const [imagePending, setImagePending] = useState(false);

  function optionalNumber(value: string) {
    return value.trim() === "" ? null : Number(value);
  }

  // Real-world plant sheets mix decimal-comma ("12,7895"), degree-symbol
  // ("101.8537°"), and compass-suffix ("12.7895 N" / "101.8537 E") formats.
  // Normalize all of them to a plain signed decimal string before Number().
  function optionalCoordinate(value: string) {
    let text = value.trim();
    if (text === "") return null;
    const compass = text.match(/([NSEW])\s*$/i)?.[1]?.toUpperCase();
    text = text.replace(/[°'"NSEWnsew]/g, "").trim();
    if (/^-?\d+,\d+$/.test(text)) text = text.replace(",", ".");
    const magnitude = Number(text);
    if (Number.isNaN(magnitude)) return NaN;
    return compass === "S" || compass === "W" ? -Math.abs(magnitude) : magnitude;
  }

  async function uploadImage(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!plant || !file) return;
    const extension = file.name.toLowerCase().split(".").pop() ?? "";
    const supportedType = ["image/png", "image/jpeg", "image/jpg", "image/webp"].includes(file.type);
    const supportedExtension = ["png", "jpg", "jpeg", "webp"].includes(extension);
    if ((!supportedType && !supportedExtension) || file.size > 2 * 1024 * 1024) {
      setError("รูปต้องเป็น PNG, JPEG หรือ WebP และมีขนาดไม่เกิน 2 MiB");
      return;
    }
    setImagePending(true);
    setError("");
    try {
      const form = new FormData();
      form.append("image", file);
      const response = await api("/api/v1/plants/" + plant.id + "/image", {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: form,
      });
      if (!response.ok) {
        const detail = (await response.text()).trim();
        throw new Error(detail === "invalid image" ? "ไฟล์รูปไม่ใช่ PNG, JPEG หรือ WebP หรือมีขนาดเกิน 2 MiB" : "ไม่สามารถอัปโหลดรูปโรงไฟฟ้าได้");
      }
      const updated = (await response.json()) as Plant;
      setImagePreview(updated.imageUrl ? assetURL(updated.imageUrl) : null);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setImagePending(false);
    }
  }

  async function removeImage() {
    if (!plant || !window.confirm("ลบรูปของโรงไฟฟ้านี้หรือไม่")) return;
    setImagePending(true);
    setError("");
    try {
      const response = await api("/api/v1/plants/" + plant.id + "/image", {
        method: "DELETE",
        headers: { "X-CSRF-Token": csrfToken() },
      });
      if (!response.ok) throw new Error("ไม่สามารถลบรูปโรงไฟฟ้าได้");
      setImagePreview(null);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setImagePending(false);
    }
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    const parsedLatitude = optionalCoordinate(latitude);
    const parsedLongitude = optionalCoordinate(longitude);
    if (Number.isNaN(parsedLatitude) || Number.isNaN(parsedLongitude)) {
      setError("รูปแบบ Latitude/Longitude ไม่ถูกต้อง");
      return;
    }
    setPending(true);
    setError("");
    const body = {
      ...(!plant ? { organizationId } : {}),
      code,
      name,
      timezone,
      latitude: parsedLatitude,
      longitude: parsedLongitude,
      installedDcKw: optionalNumber(installedDcKw),
      installedAcKw: optionalNumber(installedAcKw),
      ...(plant ? { isActive } : {}),
    };
    try {
      const response = await api(plant ? `/api/v1/plants/${plant.id}` : "/api/v1/plants", {
        method: plant ? "PUT" : "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify(body),
      });
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์เปลี่ยนข้อมูลโรงไฟฟ้า");
      if (response.status === 409) throw new Error("รหัสโรงไฟฟ้านี้ถูกใช้งานแล้วในองค์กร");
      if (!response.ok) throw new Error("ข้อมูลไม่ถูกต้องหรือไม่สามารถบันทึกได้");
      onSaved();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !pending && !imagePending) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div><DialogDescription>Plant registry</DialogDescription><DialogTitle>{plant ? "แก้ไขโรงไฟฟ้า" : "เพิ่มโรงไฟฟ้า"}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={submit}>
            {!plant && <label className="full-field">Organization ID<TextInput autoFocus value={organizationId} onChange={(event) => setOrganizationId(event.target.value)} required /></label>}
            <label>รหัสโรงไฟฟ้า<TextInput autoFocus={Boolean(plant)} value={code} onChange={(event) => setCode(event.target.value)} maxLength={100} required /></label>
            <label>ชื่อโรงไฟฟ้า<TextInput value={name} onChange={(event) => setName(event.target.value)} maxLength={200} required /></label>
            <label className="full-field">Timezone<TextInput value={timezone} onChange={(event) => setTimezone(event.target.value)} maxLength={100} placeholder="ไม่กรอก = Asia/Bangkok" /></label>
            <label>Latitude<TextInput type="text" inputMode="decimal" placeholder="12.789507 หรือ 12.789507 N" value={latitude} onChange={(event) => setLatitude(event.target.value)} /></label>
            <label>Longitude<TextInput type="text" inputMode="decimal" placeholder="101.853718 หรือ 101.853718 E" value={longitude} onChange={(event) => setLongitude(event.target.value)} /></label>
            <label>Installed DC (kW)<TextInput type="number" min="0" step="any" value={installedDcKw} onChange={(event) => setInstalledDcKw(event.target.value)} /></label>
            <label>Installed AC (kW)<TextInput type="number" min="0" step="any" value={installedAcKw} onChange={(event) => setInstalledAcKw(event.target.value)} /></label>
            {plant && <div className="plant-image-field full-field">
              <span className="field-label">รูปโรงไฟฟ้า</span>
              {imagePreview ? <img className="plant-image-preview" src={imagePreview} alt={"รูป " + plant.name} /> : <div className="plant-image-fallback">ยังไม่มีรูป</div>}
              <div className="plant-image-actions">
                <label className="button secondary-button">เลือก/เปลี่ยนรูป<input type="file" accept="image/png,image/jpeg,image/webp" hidden onChange={uploadImage} disabled={imagePending} /></label>
                {imagePreview && <Button type="button" variant="secondary" onClick={() => void removeImage()} disabled={imagePending}>ลบรูป</Button>}
              </div>
              <small>PNG, JPEG หรือ WebP ไม่เกิน 2 MiB</small>
            </div>}
            {plant && <label className="toggle-field full-field"><Checkbox checked={isActive} onChange={setIsActive} /><span>เปิดใช้งานโรงไฟฟ้า</span></label>}
            {error && <FormMessage className="full-field">{error}</FormMessage>}
            <div className="editor-actions full-field"><Button type="button" variant="secondary" onClick={onClose} disabled={pending}>ยกเลิก</Button><Button disabled={pending}>{pending ? "กำลังบันทึก" : "บันทึก"}</Button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
