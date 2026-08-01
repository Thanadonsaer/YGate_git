"use client";

import { ArchiveX, ArrowLeft, Cpu, Eye, MapPin, Pencil, PlugZap, Plus, RefreshCw, Trash2, Wifi } from "lucide-react";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, csrfToken, formatDate } from "../../lib/api";
import { useRealtimeSocket } from "../../lib/realtime";
import type { Device, DeviceModelOption, LatestTelemetry, Plant } from "../../lib/types";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { toast } from "../../components/ui/sonner";

export function PlantsPage({ defaultOrganizationId }: { defaultOrganizationId?: string }) {
  const [plants, setPlants] = useState<Plant[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<Plant | "create" | null>(null);
  const [selectedPlant, setSelectedPlant] = useState<Plant | null>(null);

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
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadPlants(); }, [loadPlants]);

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

  if (selectedPlant) {
    return <DeviceManagement plant={selectedPlant} onBack={() => setSelectedPlant(null)} />;
  }

  return (
    <div className="content plants-content">
      <div className="section-heading">
        <div><p>Plant registry</p><h2>โรงไฟฟ้าทั้งหมด</h2></div>
        <div className="heading-actions">
          <button className="icon-button" onClick={() => void loadPlants()} title="รีเฟรช" aria-label="รีเฟรชรายการโรงไฟฟ้า"><RefreshCw size={18} /></button>
          <button className="primary-button compact" onClick={() => setEditor("create")}><Plus size={18} /> เพิ่มโรงไฟฟ้า</button>
        </div>
      </div>
      {error && <p className="form-message error">{error}</p>}
      <div className="plant-table" role="table" aria-label="โรงไฟฟ้า">
        <div className="plant-row plant-head" role="row">
          <span>โรงไฟฟ้า</span><span>องค์กร</span><span>กำลังติดตั้ง</span><span>สถานะ</span><span aria-label="คำสั่ง" />
        </div>
        {!loading && plants.map((plant) => (
          <div className="plant-row" role="row" key={plant.id}>
            <div><strong>{plant.name}</strong><small><MapPin size={13} /> {plant.code} · {plant.timezone}</small></div>
            <div><span>{plant.organizationName}</span><small>{plant.organizationId}</small></div>
            <div><span>{plant.installedDcKw == null ? "-" : `${plant.installedDcKw.toLocaleString()} kWdc`}</span><small>{plant.installedAcKw == null ? "ไม่ระบุ AC" : `${plant.installedAcKw.toLocaleString()} kWac`}</small></div>
            <span className={plant.isActive ? "status active" : "status revoked"}>{plant.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</span>
            <div className="row-actions">
              <button className="icon-button" onClick={() => setSelectedPlant(plant)} title="จัดการ Device" aria-label={`จัดการ Device ใน ${plant.name}`}><Cpu size={17} /></button>
              <button className="icon-button" onClick={() => setEditor(plant)} title="แก้ไขโรงไฟฟ้า" aria-label={`แก้ไข ${plant.name}`}><Pencil size={17} /></button>
              {plant.isActive && <button className="icon-button" onClick={() => void decommissionPlant(plant)} title="ปิดใช้งาน" aria-label={`ปิดใช้งาน ${plant.name}`}><ArchiveX size={17} /></button>}
              <button className="icon-button danger" onClick={() => void hardDeletePlant(plant)} title="ลบถาวร (Platform Admin)" aria-label={`ลบ ${plant.name} ถาวร`}><Trash2 size={17} /></button>
            </div>
          </div>
        ))}
        {loading && <div className="table-state">กำลังโหลดข้อมูล</div>}
        {!loading && !error && plants.length === 0 && <div className="table-state">ยังไม่มีโรงไฟฟ้าในขอบเขตที่คุณเข้าถึงได้</div>}
      </div>
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
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
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
        modbusPort: device.modbusPort ?? null, modbusUnitId: device.modbusUnitId, pollIntervalSeconds: device.pollIntervalSeconds,
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
      const data = (await response.json()) as { ok?: boolean; error?: string };
      const message = data.error || (kind === "test-connection" ? "เชื่อมต่อสำเร็จ" : "อ่านค่าสำเร็จ");
      setTestOutcomes((prev) => ({ ...prev, [device.id]: { pending: false } }));
      if (data.ok === false) toast.error(message); else toast.success(message);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด";
      setTestOutcomes((prev) => ({ ...prev, [device.id]: { pending: false } }));
      toast.error(message);
    }
  }

  const latestReadings = Object.values(latestByDevice);
  const lastObservedAt = latestReadings.reduce<string | undefined>((latest, reading) => !latest || reading.observedAt > latest ? reading.observedAt : latest, undefined);

  return (
    <div className="content devices-content">
      <div className="section-heading">
        <div className="registry-title">
          <button className="icon-button" onClick={onBack} title="กลับไปโรงไฟฟ้า" aria-label="กลับไปโรงไฟฟ้า"><ArrowLeft size={18} /></button>
          <div><p>{plant.code}</p><h2>Device ใน {plant.name}</h2></div>
        </div>
        <div className="row-actions">
          <span className={`live-chip ${liveState}`}><Wifi size={15} />{liveState === "connected" ? "Live data" : liveState === "connecting" ? "Connecting" : "Offline"}</span>
          <button className="icon-button" onClick={() => void loadDevices()} title="รีเฟรช" aria-label="รีเฟรชรายการ Device"><RefreshCw size={18} /></button>
          <button className="primary-button compact" onClick={() => setEditor("create")}><Plus size={18} /> เพิ่ม Device</button>
        </div>
      </div>
      {error && <p className="form-message error">{error}</p>}
      <section className="grid gap-px overflow-hidden rounded-md border border-slate-200 bg-slate-200 sm:grid-cols-2 xl:grid-cols-4" aria-label="Plant summary">
        <div className="bg-white p-4"><small className="font-bold text-slate-500">Installed capacity</small><strong className="mt-1 block text-xl text-slate-900">{plant.installedDcKw?.toLocaleString() ?? "-"} kWdc</strong><span className="text-xs text-slate-500">{plant.installedAcKw?.toLocaleString() ?? "-"} kWac</span></div>
        <div className="bg-white p-4"><small className="font-bold text-slate-500">Devices</small><strong className="mt-1 block text-xl text-slate-900">{devices.length.toLocaleString()}</strong><span className="text-xs text-slate-500">ใช้งาน {devices.filter((device) => device.isActive).length.toLocaleString()}</span></div>
        <div className="bg-white p-4"><small className="font-bold text-slate-500">Reporting</small><strong className="mt-1 block text-xl text-slate-900">{latestReadings.length.toLocaleString()}</strong><span className="text-xs text-slate-500">Device ที่มีค่าล่าสุด</span></div>
        <div className="bg-white p-4"><small className="font-bold text-slate-500">Latest observed</small><strong className="mt-1 block text-sm text-slate-900">{lastObservedAt ? formatDate(lastObservedAt) : "ยังไม่มีข้อมูล"}</strong><span className="text-xs text-slate-500">Timezone: {plant.timezone}</span></div>
      </section>
      <div className="device-table" role="table" aria-label={`Device ใน ${plant.name}`}>
        <div className="device-row device-head" role="row">
          <span>Device</span><span>รุ่น</span><span>ประเภท</span><span>ค่าล่าสุด</span><span>สถานะ</span><span aria-label="คำสั่ง" />
        </div>
        {!loading && devices.map((device) => {
          const outcome = testOutcomes[device.id];
          const canTest = Boolean(device.modbusHost && device.modbusPort);
          return (
            <div className="device-row" role="row" key={device.id}>
              <div><strong>{device.name}</strong><small>{device.externalId}</small></div>
              <div><span>{device.model}</span><small>{device.manufacturer}</small></div>
              <div><span>{device.modbusHost ? `${device.modbusHost}:${device.modbusPort}` : "ไม่ใช่ Modbus device"}</span><small>{device.modbusHost ? `unit ${device.modbusUnitId} · poll ${device.pollIntervalSeconds}s` : device.deviceType}</small></div>
              <LatestValues reading={latestByDevice[device.id]} />
              <span className={device.isActive ? "status active" : "status revoked"}>{device.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</span>
              <div className="row-actions">
                <button className="icon-button" disabled={!canTest || outcome?.pending} onClick={() => void runCommand("test-connection", device)} title={canTest ? "ทดสอบการเชื่อมต่อ" : "ต้องตั้งค่า IP/Port ก่อน"} aria-label={`ทดสอบการเชื่อมต่อ ${device.name}`}><PlugZap size={17} /></button>
                <button className="icon-button" disabled={!canTest || outcome?.pending} onClick={() => void runCommand("test-read", device)} title="ทดสอบอ่านค่า" aria-label={`ทดสอบอ่านค่า ${device.name}`}><RefreshCw size={17} /></button>
                <button className="icon-button" onClick={() => setSelectedDevice(device)} title="ดูค่าล่าสุดทั้งหมด" aria-label={`ดูค่าล่าสุดของ ${device.name}`}><Eye size={17} /></button>
                <button className="icon-button" onClick={() => setEditor(device)} title="แก้ไข Device" aria-label={`แก้ไข ${device.name}`}><Pencil size={17} /></button>
                {device.isActive && <button className="icon-button" onClick={() => void decommissionDevice(device)} title="ปิดใช้งาน" aria-label={`ปิดใช้งาน ${device.name}`}><ArchiveX size={17} /></button>}
                <button className="icon-button danger" onClick={() => void hardDeleteDevice(device)} title="ลบถาวร (Platform Admin)" aria-label={`ลบ ${device.name} ถาวร`}><Trash2 size={17} /></button>
              </div>
            </div>
          );
        })}
        {loading && <div className="table-state">กำลังโหลดข้อมูล</div>}
        {!loading && !error && devices.length === 0 && <div className="table-state">ยังไม่มี Device กดเพิ่ม Device หรือให้ Middleware auto onboard เมื่อส่งข้อมูลเข้ามา</div>}
      </div>
      {selectedDevice && <DeviceLatestDialog device={selectedDevice} reading={latestByDevice[selectedDevice.id]} onClose={() => setSelectedDevice(null)} />}
      {editor && <DeviceEditor plant={plant} device={editor === "create" ? undefined : editor} onClose={() => setEditor(null)} onSaved={() => { setEditor(null); void loadDevices(); }} />}
    </div>
  );
}

function DeviceLatestDialog({ device, reading, onClose }: { device: Device; reading?: LatestTelemetry; onClose: () => void }) {
  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div><DialogDescription>{device.externalId}</DialogDescription><DialogTitle>ค่าล่าสุดของ {device.name}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          {!reading ? <div className="table-state">ยังไม่มี telemetry สำหรับ Device นี้</div> : <>
            <div className="grid grid-cols-2 gap-3 px-5 py-4 text-sm"><div><small className="block text-slate-500">Observed</small><strong>{formatDate(reading.observedAt)}</strong></div><div><small className="block text-slate-500">Received</small><strong>{formatDate(reading.receivedAt)}</strong></div></div>
            <div className="max-h-[55vh] overflow-auto border-t border-slate-200 px-5 py-3">
              {Object.entries(reading.dataItemMap).sort(([a], [b]) => a.localeCompare(b)).map(([key, value]) => <div key={key} className="flex items-center justify-between gap-4 border-b border-slate-100 py-2 text-sm last:border-0"><code className="text-slate-600">{key}</code><strong className="text-slate-900">{Number.isFinite(value) ? value.toLocaleString() : "-"}</strong></div>)}
            </div>
          </>}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

function LatestValues({ reading }: { reading?: LatestTelemetry }) {
  if (!reading) return <div><span>-</span><small>ยังไม่มี telemetry</small></div>;
  const values = Object.entries(reading.dataItemMap).slice(0, 3);
  return (
    <div className="latest-values" title={`รับเมื่อ ${formatDate(reading.receivedAt)}`}>
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
  const [pollIntervalSeconds, setPollIntervalSeconds] = useState(device?.pollIntervalSeconds?.toString() ?? "10");
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
      externalId, name, deviceModelId,
      modbusHost: host,
      modbusPort: host === "" ? null : Number(modbusPort),
      modbusUnitId: Number(modbusUnitId),
      pollIntervalSeconds: Number(pollIntervalSeconds),
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
      if (!response.ok) throw new Error("ข้อมูลไม่ถูกต้องหรือไม่สามารถบันทึกได้ (Model ต้องเลือก, IP ต้องมี Port คู่กัน)");
      onSaved();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
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
            <label>Device ID<input autoFocus={!device} value={externalId} onChange={(event) => setExternalId(event.target.value)} maxLength={200} required disabled={Boolean(device)} /></label>
            <label>ชื่อ Device<input autoFocus={Boolean(device)} value={name} onChange={(event) => setName(event.target.value)} maxLength={200} required /></label>
            <label className="full-field">Model
              <Select value={deviceModelId} onValueChange={setDeviceModelId} disabled={models.length === 0}>
                <SelectTrigger><SelectValue placeholder="ยังไม่มี Device Model — สร้างที่หน้า Register Metadata ก่อน" /></SelectTrigger>
                <SelectContent>{models.map((m) => <SelectItem key={m.id} value={m.id}>{m.manufacturer} / {m.deviceType} / {m.model}</SelectItem>)}</SelectContent>
              </Select>
            </label>
            <label>IP<input value={modbusHost} onChange={(event) => setModbusHost(event.target.value)} placeholder="192.168.1.100 (เว้นว่างถ้าไม่ใช่ Modbus device)" /></label>
            <label>Port<input type="number" min="1" max="65535" value={modbusPort} onChange={(event) => setModbusPort(event.target.value)} /></label>
            <label>Unit ID<input type="number" min="0" max="255" value={modbusUnitId} onChange={(event) => setModbusUnitId(event.target.value)} /></label>
            <label>Poll interval (s)<input type="number" min="1" max="3600" value={pollIntervalSeconds} onChange={(event) => setPollIntervalSeconds(event.target.value)} /></label>
            <label className="toggle-field full-field"><input type="checkbox" checked={isActive} onChange={(event) => setIsActive(event.target.checked)} /><span>เปิดใช้งาน Device</span></label>
            {error && <p className="form-message error full-field">{error}</p>}
            <div className="editor-actions full-field"><button type="button" className="secondary-button" onClick={onClose} disabled={pending}>ยกเลิก</button><button className="primary-button" disabled={pending}>{pending ? "กำลังบันทึก" : device ? "บันทึก" : "สร้าง Device"}</button></div>
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

  function optionalNumber(value: string) {
    return value.trim() === "" ? null : Number(value);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    const body = {
      ...(!plant ? { organizationId } : {}),
      code,
      name,
      timezone,
      latitude: optionalNumber(latitude),
      longitude: optionalNumber(longitude),
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
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !pending) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div><DialogDescription>Plant registry</DialogDescription><DialogTitle>{plant ? "แก้ไขโรงไฟฟ้า" : "เพิ่มโรงไฟฟ้า"}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={submit}>
            {!plant && <label className="full-field">Organization ID<input autoFocus value={organizationId} onChange={(event) => setOrganizationId(event.target.value)} required /></label>}
            <label>รหัสโรงไฟฟ้า<input autoFocus={Boolean(plant)} value={code} onChange={(event) => setCode(event.target.value)} maxLength={100} required /></label>
            <label>ชื่อโรงไฟฟ้า<input value={name} onChange={(event) => setName(event.target.value)} maxLength={200} required /></label>
            <label className="full-field">Timezone<input value={timezone} onChange={(event) => setTimezone(event.target.value)} maxLength={100} required /></label>
            <label>Latitude<input type="number" min="-90" max="90" step="any" value={latitude} onChange={(event) => setLatitude(event.target.value)} /></label>
            <label>Longitude<input type="number" min="-180" max="180" step="any" value={longitude} onChange={(event) => setLongitude(event.target.value)} /></label>
            <label>Installed DC (kW)<input type="number" min="0" step="any" value={installedDcKw} onChange={(event) => setInstalledDcKw(event.target.value)} /></label>
            <label>Installed AC (kW)<input type="number" min="0" step="any" value={installedAcKw} onChange={(event) => setInstalledAcKw(event.target.value)} /></label>
            {plant && <label className="toggle-field full-field"><input type="checkbox" checked={isActive} onChange={(event) => setIsActive(event.target.checked)} /><span>เปิดใช้งานโรงไฟฟ้า</span></label>}
            {error && <p className="form-message error full-field">{error}</p>}
            <div className="editor-actions full-field"><button type="button" className="secondary-button" onClick={onClose} disabled={pending}>ยกเลิก</button><button className="primary-button" disabled={pending}>{pending ? "กำลังบันทึก" : "บันทึก"}</button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
