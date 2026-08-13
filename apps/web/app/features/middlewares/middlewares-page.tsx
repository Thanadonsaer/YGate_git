"use client";

import { ArchiveX, ArrowDownToLine, ArrowLeft, ArrowRight, ArrowUpToLine, CheckCircle2, FileUp, Loader2, Pencil, Plus, RefreshCw, RotateCcw, Save, Search, Settings2, Trash2, X } from "lucide-react";
import { Checkbox, FormMessage, StatusTag, TextInput } from "../../components/ui/form";
import { type ReactNode, FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { api, errorMessage, csrfToken } from "../../lib/api";
import { inputClass } from "../../components/ui";
import { cn } from "../../lib/cn";
import type { CreatedMiddlewareGateway, ImportMiddlewareConfigResult, MiddlewareConfigSnapshot, MiddlewareConnection, MiddlewareGateway, MiddlewarePatch, Plant } from "../../lib/types";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { DataTable, TableColumn } from "../../components/ui/data-table";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";
import { middlewareProgressLabel } from "../../lib/middleware-progress";

function ProgressBar({ label }: { label: string }) {
  return (
    <div className="progress-status" role="status" aria-live="polite">
      <div className="progress-status-head"><span>{label}</span><Loader2 size={14} className="animate-spin" /></div>
      <div className="progress-status-track" aria-hidden="true"><span /></div>
    </div>
  );
}

export function MiddlewaresPage({ defaultOrganizationId }: { defaultOrganizationId?: string }) {
  const [gateways, setGateways] = useState<MiddlewareGateway[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<MiddlewareGateway | "create" | null>(null);
  const [createdKey, setCreatedKey] = useState<CreatedMiddlewareGateway | null>(null);
  const [selected, setSelected] = useState<MiddlewareGateway | null>(null);
  const [search, setSearch] = useState("");

  const loadGateways = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await api("/api/v1/admin/middlewares");
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์จัดการ Middleware");
      if (!response.ok) throw new Error("ไม่สามารถโหลดรายการ Middleware ได้");
      setGateways((await response.json()) as MiddlewareGateway[]);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadGateways(); }, [loadGateways]);

  const filteredGateways = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return gateways;
    return gateways.filter((gateway) =>
      gateway.name.toLowerCase().includes(q)
      || gateway.siteName?.toLowerCase().includes(q)
      || gateway.organizationName?.toLowerCase().includes(q)
      || gateway.id.toLowerCase().includes(q),
    );
  }, [gateways, search]);

  async function setGatewayActive(gateway: MiddlewareGateway, isActive: boolean) {
    if (!isActive && !window.confirm(`ปิดใช้งาน Middleware "${gateway.name}"? Gateway นี้จะเชื่อมต่อ realtime ไม่ได้ทันที`)) return;
    const response = await api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}`, {
      method: "PUT",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ name: gateway.name, siteName: gateway.siteName, autoOnboard: gateway.autoOnboard, isActive }),
    });
    if (response.ok) { toast.success(isActive ? `เปิดใช้งาน "${gateway.name}" แล้ว` : `ปิดใช้งาน "${gateway.name}" แล้ว`); await loadGateways(); }
    else setError(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์เปลี่ยนสถานะ Middleware" : "ไม่สามารถเปลี่ยนสถานะ Middleware ได้");
  }

  async function hardDeleteGateway(gateway: MiddlewareGateway) {
    const expected = "DELETE";
    if (window.prompt(`คำสั่งนี้จะลบ Middleware Gateway และ ingestion history ของ client นี้ถาวร\nพิมพ์ ${expected}`) !== expected) return;
    // Deletes by id from the shared middleware_client row, so the same endpoint
    // the old API Keys page used still applies here — see platform-api's
    // hardDeleteAPIKeyHandler / HardDeleteAPIKey.
    const response = await api(`/api/v1/admin/api-keys/${encodeURIComponent(gateway.id)}`, {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrfToken(), "X-Hard-Delete-Confirm": expected },
    });
    if (response.ok) { toast.success(`ลบ Middleware "${gateway.name}" ถาวรแล้ว`); await loadGateways(); }
    else setError(response.status === 403 ? "เฉพาะ System Admin เท่านั้นที่ลบ Middleware ถาวรได้" : "ไม่สามารถลบ Middleware ถาวรได้");
  }

  if (selected) {
    return <MiddlewareConfigEditor gateway={selected} onBack={() => setSelected(null)} />;
  }

  return (
    <div className="content">
      <div className="section-heading">
        <div><p>Site gateways</p><h2>Middleware Gateways</h2></div>
        <div className="heading-actions">
          <Button variant="icon" onClick={() => void loadGateways()} title="รีเฟรช" aria-label="รีเฟรช Middleware"><RefreshCw size={18} /></Button>
          <Button compact onClick={() => setEditor("create")}><Plus size={18} /> เพิ่ม Middleware</Button>
        </div>
      </div>
      {createdKey && (
        <section className="mb-4 grid grid-cols-[minmax(180px,.8fr)_minmax(0,1.4fr)_auto_36px] items-center gap-3 border border-[#bce6d4] bg-[#ebf8f2] p-3.5">
          <div className="grid min-w-0 gap-1">
            <strong className="min-w-0 truncate text-ink">API key ใหม่สำหรับ {createdKey.name}</strong>
            <small className="min-w-0 truncate text-[11px] text-[#0d6744]">ระบบจะแสดง key เต็มเฉพาะครั้งนี้ ใช้ตั้งค่าที่ site ด้วย -api-key flag หรือ gateway-config</small>
          </div>
          <code className="min-w-0 overflow-auto whitespace-nowrap rounded-[var(--radius-sm)] border border-[#bce6d4] bg-white px-2.5 py-2 font-mono text-xs text-[#0d3f2d]">{createdKey.apiKey}</code>
          <Button variant="secondary" compact onClick={() => void navigator.clipboard.writeText(createdKey.apiKey)}>คัดลอก</Button>
          <Button variant="icon" onClick={() => setCreatedKey(null)} title="ปิด" aria-label="ปิดข้อความ API key"><X size={17} /></Button>
        </section>
      )}
      {error && <FormMessage>{error}</FormMessage>}
      {loading && <ProgressBar label="กำลังโหลด Middleware..." />}
      {loading ? (
        <div className="table-state">กำลังโหลด Middleware</div>
      ) : (
        <>
          <label className="relative mb-3 block w-full max-w-sm">
            <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-soft" size={16} />
            <span className="sr-only">ค้นหา Middleware</span>
            <TextInput className={`${inputClass} pl-9`} type="search" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="ค้นหา Middleware ด้วยชื่อ, Site หรือ Organization" />
          </label>
          <DataTable value={filteredGateways} dataKey="id" aria-label="Middleware Gateways" emptyMessage={<div className="table-state">{gateways.length === 0 ? "ยังไม่มี Middleware Gateway" : "ไม่พบ Middleware ที่ค้นหา"}</div>}>
            <TableColumn field="name" header="Gateway" sortable body={(row: MiddlewareGateway) => (
              <div className="grid min-w-0 gap-1"><strong className="truncate text-ink">{row.name}</strong><small className="truncate text-[11px] text-ink-soft">{row.id}</small></div>
            )} />
            <TableColumn field="siteName" header="Site" sortable body={(row: MiddlewareGateway) => (
              <div className="grid min-w-0 gap-1"><span className="truncate">{row.siteName || "-"}</span><small className="truncate text-[11px] text-ink-soft">{row.organizationName}</small></div>
            )} />
            <TableColumn header="Key" body={(row: MiddlewareGateway) => (
              <div className="grid min-w-0 gap-1"><span className="truncate">{row.keyPrefix}...</span><small className="truncate text-[11px] text-ink-soft">ไม่แสดง secret หลังสร้าง</small></div>
            )} />
            <TableColumn field="autoOnboard" header="Auto onboard" sortable body={(row: MiddlewareGateway) => <StatusTag tone={row.autoOnboard ? "active" : "revoked"}>{row.autoOnboard ? "เปิด" : "ปิด"}</StatusTag>} />
            <TableColumn field="isOnline" header="เชื่อมต่อ" sortable body={(row: MiddlewareGateway) => <StatusTag tone={row.isOnline ? "active" : "revoked"}>{row.isOnline ? "Online" : "Offline"}</StatusTag>} />
            <TableColumn field="configVersion" header="Config version" sortable body={(row: MiddlewareGateway) => (
              <div className="grid min-w-0 gap-1"><span>v{row.configAppliedVersion} / v{row.configVersion}</span><small className="text-[11px] text-ink-soft">{row.configAppliedVersion < row.configVersion ? "รอ push ไป gateway" : "อัปเดตล่าสุดแล้ว"}</small></div>
            )} />
            <TableColumn field="isActive" header="สถานะ" sortable body={(row: MiddlewareGateway) => <StatusTag tone={row.isActive ? "active" : "revoked"}>{row.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</StatusTag>} />
            <TableColumn header="คำสั่ง" body={(row: MiddlewareGateway) => (
              <div className="row-actions">
                <Button variant="icon" onClick={() => setSelected(row)} title="ตั้งค่า Config" aria-label={`ตั้งค่า Config ของ ${row.name}`}><Settings2 size={17} /></Button>
                <Button variant="icon" onClick={() => setEditor(row)} title="แก้ไข" aria-label={`แก้ไข ${row.name}`}><Pencil size={17} /></Button>
                <Button variant="icon" onClick={() => void setGatewayActive(row, !row.isActive)} title={row.isActive ? "ปิดใช้งาน" : "เปิดใช้งาน"} aria-label={row.isActive ? `ปิดใช้งาน ${row.name}` : `เปิดใช้งาน ${row.name}`}>
                  {row.isActive ? <ArchiveX size={17} /> : <CheckCircle2 size={17} />}
                </Button>
                <Button variant="icon" danger onClick={() => void hardDeleteGateway(row)} title="ลบถาวร (System Admin)" aria-label={`ลบ ${row.name} ถาวร`}><Trash2 size={17} /></Button>
              </div>
            )} />
          </DataTable>
        </>
      )}
      {editor && (
        <MiddlewareEditor
          gateway={editor === "create" ? undefined : editor}
          defaultOrganizationId={defaultOrganizationId}
          onClose={() => setEditor(null)}
          onSaved={(created) => { setEditor(null); if (created) setCreatedKey(created); void loadGateways(); }}
        />
      )}
    </div>
  );
}

function MiddlewareEditor({ gateway, defaultOrganizationId, onClose, onSaved }: { gateway?: MiddlewareGateway; defaultOrganizationId?: string; onClose: () => void; onSaved: (created?: CreatedMiddlewareGateway) => void }) {
  const [organizationId, setOrganizationId] = useState(gateway?.organizationId ?? defaultOrganizationId ?? "");
  const [name, setName] = useState(gateway?.name ?? "");
  const [siteName, setSiteName] = useState(gateway?.siteName ?? "");
  const [autoOnboard, setAutoOnboard] = useState(gateway?.autoOnboard ?? true);
  const [pollIntervalMinutes, setPollIntervalMinutes] = useState(gateway ? Math.max(1, Math.round(gateway.pollIntervalSeconds / 60)).toString() : "1");
  const [idleHeartbeatMinutes, setIdleHeartbeatMinutes] = useState(gateway ? Math.max(1, Math.round(gateway.idleHeartbeatSeconds / 60)).toString() : "30");
  const [apiPollingEnabled, setApiPollingEnabled] = useState(gateway?.apiPollingEnabled ?? false);
  const [isActive, setIsActive] = useState(gateway?.isActive ?? true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const pollIntervalSeconds = Number(pollIntervalMinutes) * 60;
      const idleHeartbeatSeconds = Number(idleHeartbeatMinutes) * 60;
      const response = await api(gateway ? `/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}` : "/api/v1/admin/middlewares", {
        method: gateway ? "PUT" : "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify(
          gateway
            ? { name, siteName, autoOnboard, isActive, pollIntervalSeconds, idleHeartbeatSeconds, apiPollingEnabled }
            : { organizationId, name, siteName, autoOnboard, pollIntervalSeconds, idleHeartbeatSeconds, apiPollingEnabled },
        ),
      });
      if (!response.ok) throw new Error(response.status === 409 ? "ชื่อ Middleware นี้มีอยู่แล้ว" : response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์จัดการ Middleware" : "ไม่สามารถบันทึก Middleware ได้");
      onSaved(gateway ? undefined : ((await response.json()) as CreatedMiddlewareGateway));
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
          <div><DialogDescription>Middleware gateway</DialogDescription><DialogTitle>{gateway ? "แก้ไข Middleware" : "เพิ่ม Middleware"}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={submit}>
            <label className="full-field">ชื่อ Gateway<TextInput autoFocus value={name} onChange={(event) => setName(event.target.value)} maxLength={200} required /></label>
            <label className="full-field">ชื่อ Site<TextInput value={siteName} onChange={(event) => setSiteName(event.target.value)} maxLength={200} placeholder="เช่น VT1 - Vientiane Solar" /></label>
            {!gateway && <label className="full-field">Organization ID<TextInput value={organizationId} onChange={(event) => setOrganizationId(event.target.value)} required /></label>}
            <label>ส่งข้อมูลทุก (นาที)<TextInput type="number" min="1" max="60" value={pollIntervalMinutes} onChange={(event) => setPollIntervalMinutes(event.target.value)} /></label>
            <label>บันทึกซ้ำเมื่อค่านิ่ง ทุก (นาที)
              <TextInput type="number" min="1" max="1440" value={idleHeartbeatMinutes} onChange={(event) => setIdleHeartbeatMinutes(event.target.value)} />
              <small className="muted-text">ถ้าค่า register ไม่เปลี่ยน Gateway จะหยุดบันทึก แล้วส่งซ้ำ 1 ครั้งทุกช่วงนี้ เพื่อให้ยังรู้ว่า Device ออนไลน์อยู่</small>
            </label>
            <label className="toggle-field full-field"><Checkbox checked={apiPollingEnabled} onChange={setApiPollingEnabled} /> เปิดใช้งาน Telemetry Pull (platform ดึงข้อมูลผ่าน WebSocket)</label>
            <label className="toggle-field full-field"><Checkbox checked={autoOnboard} onChange={setAutoOnboard} /> Auto onboard Plant/Device</label>
            {gateway && <label className="toggle-field full-field"><Checkbox checked={isActive} onChange={setIsActive} /> เปิดใช้งาน Middleware</label>}
            {error && <FormMessage className="full-field">{error}</FormMessage>}
            <div className="editor-actions full-field"><Button type="button" variant="secondary" onClick={onClose} disabled={pending}>ยกเลิก</Button><Button disabled={pending}><Save size={17} /> {pending ? "กำลังบันทึก" : "บันทึก Middleware"}</Button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

// The lifecycle endpoints (stage/apply/rollback/restart) return the real
// reason as the response body (see writeMiddlewareError in platform-api) --
// prefer that over a guess, since "offline" is only one of several distinct
// failure modes (missing server config, unsupported old middleware build,
// no staged patch, etc) that a generic message would otherwise conflate.
async function middlewareLifecycleError(response: Response, fallbackHint: string): Promise<string> {
  if (response.status === 503) return "Middleware ออฟไลน์อยู่";
  if (response.status === 504) return "Middleware เชื่อมต่ออยู่ (online) แต่ไม่ตอบสนองคำสั่งนี้ — อาจเป็น Middleware เวอร์ชันเก่าที่ยังไม่รองรับ remote update";
  const body = (await response.text()).trim();
  return body || `ดำเนินการไม่สำเร็จ (${fallbackHint})`;
}

/** Shared card shell for the single-page gateway dashboard. */
function Card({ title, subtitle, className, children }: { title: string; subtitle?: ReactNode; className?: string; children: ReactNode }) {
  return (
    <section className={cn("rounded-[var(--radius-md)] border border-line bg-surface p-4", className)}>
      <header className="mb-3">
        <h3 className="text-sm font-bold text-ink">{title}</h3>
        {subtitle && <p className="mt-1 text-xs text-ink-soft">{subtitle}</p>}
      </header>
      {children}
    </section>
  );
}

function PlantsCard({ assignedPlants, unassignedPlants, addPlantId, setAddPlantId, pending, loading, onAssign, onUnassign }: {
  assignedPlants: Plant[];
  unassignedPlants: Plant[];
  addPlantId: string;
  setAddPlantId: (value: string) => void;
  pending: boolean;
  loading: boolean;
  onAssign: () => void;
  onUnassign: (plant: Plant) => void;
}) {
  return (
    <Card
      title={`Plants (${assignedPlants.length})`}
      subtitle="เลือก Device ที่ต้อง poll ผ่านหน้า Plants → Devices ของแต่ละ Plant — config ที่ส่งไป Middleware คำนวณอัตโนมัติจาก Device ที่ตั้งค่า IP/Port ไว้แล้ว"
    >
      <ul className="m-0 mb-3 flex list-none flex-col gap-1.5 p-0">
        {assignedPlants.map((plant) => (
          <li key={plant.id} className="flex items-center justify-between gap-2 rounded-[var(--radius-sm)] border border-line bg-canvas/40 px-3 py-2 text-sm">
            <div className="min-w-0"><strong className="block truncate text-ink">{plant.name}</strong><small className="block truncate text-[11px] text-ink-soft">{plant.code} · {plant.timezone}</small></div>
            <Button variant="icon" danger disabled={pending} onClick={() => onUnassign(plant)} title="เอาออก" aria-label={`เอา ${plant.name} ออก`}><Trash2 size={16} /></Button>
          </li>
        ))}
        {!loading && assignedPlants.length === 0 && <li className="table-state">ยังไม่ได้มอบหมาย Plant ให้ Middleware นี้</li>}
      </ul>
      <div className="row-actions" style={{ justifyContent: "flex-start" }}>
        <Select value={addPlantId} onValueChange={setAddPlantId} disabled={unassignedPlants.length === 0}>
          <SelectTrigger className="w-56"><SelectValue placeholder="เลือก Plant ที่จะมอบหมาย..." /></SelectTrigger>
          <SelectContent>{unassignedPlants.map((p) => <SelectItem key={p.id} value={p.id}>{p.code} - {p.name}</SelectItem>)}</SelectContent>
        </Select>
        <Button compact disabled={!addPlantId || pending} onClick={onAssign}><Plus size={16} /> เพิ่ม Plant</Button>
      </div>
    </Card>
  );
}

function ConfigCard({ isOnline, pushing, importing, onPush, onImport }: { isOnline: boolean; pushing: boolean; importing: boolean; onPush: () => void; onImport: () => void }) {
  return (
    <Card title="Push / Pull Config" subtitle={'"ส่ง Config" เขียนทับค่าบน Middleware ด้วยค่าจาก ygate — "ดึง Config" อ่านค่าเดิมจาก Middleware เข้ามาไว้ใน ygate (ใช้ตอน onboard Middleware เก่า) — ไม่ส่งอัตโนมัติ ต้องกดเอง'}>
      <div className="flex flex-col gap-2">
        <Button variant="secondary" className="items-center justify-start text-left" disabled={pushing || importing} onClick={onPush}>
          {pushing ? <Loader2 size={17} className="animate-spin shrink-0" /> : <ArrowUpToLine size={17} className="shrink-0" />}
          <span className="flex flex-col items-start">
            <span>{pushing ? "กำลังส่ง Config..." : "ส่ง Config ไปที่ Middleware"}</span>
            <span className="text-[11px] font-normal text-ink-soft">เขียนทับค่าเดิมบน Middleware ด้วยค่าจาก ygate</span>
          </span>
        </Button>
        <Button variant="secondary" className="items-center justify-start text-left" disabled={!isOnline || importing || pushing} onClick={onImport}>
          {importing ? <Loader2 size={17} className="animate-spin shrink-0" /> : <ArrowDownToLine size={17} className="shrink-0" />}
          <span className="flex flex-col items-start">
            <span>ดึง Config จาก Middleware</span>
            <span className="text-[11px] font-normal text-ink-soft">{!isOnline ? "Middleware ต้อง Online ก่อนจึงจะดึงได้" : importing ? "กำลังดึง Config..." : "อ่านค่าเดิมจาก Middleware เข้ามาไว้ใน ygate"}</span>
          </span>
        </Button>
      </div>
    </Card>
  );
}

function SoftwareCard({ softwareVersion, patches, selectedPatchId, setSelectedPatchId, lifecycleBusy, uploading, staging, applying, rollingBack, restarting, onUpload, onStage, onApply, onRollback, onDeletePatch, onRestart }: {
  softwareVersion?: string | null;
  patches: MiddlewarePatch[];
  selectedPatchId: string;
  setSelectedPatchId: (value: string) => void;
  lifecycleBusy: boolean;
  uploading: boolean;
  staging: boolean;
  applying: boolean;
  rollingBack: boolean;
  restarting: boolean;
  onUpload: (file: File) => void;
  onStage: () => void;
  onApply: () => void;
  onRollback: () => void;
  onDeletePatch: () => void;
  onRestart: () => void;
}) {
  const progressLabel = middlewareProgressLabel({ uploading, staging, applying, rollingBack, restarting });
  return (
    <Card title="Software Update" subtitle={`Version ปัจจุบัน: ${softwareVersion || "ไม่ทราบ (Middleware ยังไม่เคยส่ง version มา)"} — upload patch, stage ไปที่เครื่องนี้, แล้วค่อย Apply แยกกัน (ไม่ auto)`}>
      <div className="flex flex-col gap-3">
        {progressLabel && <ProgressBar label={progressLabel} />}
        <div className="row-actions" style={{ justifyContent: "flex-start" }}>
          <Select value={selectedPatchId} onValueChange={setSelectedPatchId} disabled={patches.length === 0}>
            <SelectTrigger className="w-56"><SelectValue placeholder="เลือก Patch..." /></SelectTrigger>
            <SelectContent>{patches.map((p) => <SelectItem key={p.id} value={p.id}>{p.version} ({p.os}/{p.arch})</SelectItem>)}</SelectContent>
          </Select>
          <label
            className={cn(
              "inline-flex cursor-pointer items-center justify-center gap-1.5 rounded-[var(--radius-md)] border border-line bg-surface px-2.5 py-1.5 text-xs font-bold text-ink transition hover:bg-canvas focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus",
              lifecycleBusy && "pointer-events-none opacity-48",
            )}
          >
            {uploading ? <Loader2 size={16} className="animate-spin" /> : <FileUp size={16} />}
            Upload Patch (.zip)
            <input
              type="file"
              accept=".zip"
              disabled={lifecycleBusy}
              className="hidden"
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (file) onUpload(file);
              }}
            />
          </label>
          <Button variant="text" compact danger disabled={!selectedPatchId || lifecycleBusy} onClick={onDeletePatch}>ลบ Patch</Button>
        </div>

        <div className="flex flex-wrap items-center gap-2 rounded-[var(--radius-sm)] border border-dashed border-line bg-canvas/40 p-2.5">
          <span className="grid h-5 w-5 shrink-0 place-items-center rounded-full bg-ink-soft text-[10px] font-bold text-white">1</span>
          <Button variant="secondary" compact disabled={!selectedPatchId || lifecycleBusy} onClick={onStage}>{staging ? "กำลัง Stage..." : "Stage"}</Button>
          <ArrowRight size={14} className="shrink-0 text-ink-soft" />
          <span className="grid h-5 w-5 shrink-0 place-items-center rounded-full bg-ink-soft text-[10px] font-bold text-white">2</span>
          <Button compact disabled={lifecycleBusy} onClick={onApply}>{applying ? "กำลัง Apply..." : "Apply"}</Button>
          <span className="text-[11px] text-ink-soft">ต้อง Stage patch ก่อน — Apply จะ restart service</span>
        </div>

        <div className="row-actions" style={{ justifyContent: "flex-start" }}>
          <Button variant="text" compact disabled={lifecycleBusy} onClick={onRollback}>{rollingBack ? "กำลัง Rollback..." : "Rollback ไป version ก่อนหน้า"}</Button>
          <Button variant="text" compact disabled={lifecycleBusy} onClick={onRestart}>
            {restarting ? <Loader2 size={15} className="animate-spin" /> : <RotateCcw size={15} />} {restarting ? "กำลัง Restart..." : "Restart Service"}
          </Button>
        </div>
      </div>
    </Card>
  );
}

function ConnectionsCard({ snapshot }: { snapshot: MiddlewareConfigSnapshot | null }) {
  const connections = snapshot?.connections ?? [];
  return (
    <Card
      title={`Connections${snapshot ? ` (v${snapshot.version})` : ""}`}
      subtitle="อ่านอย่างเดียว — มาจาก Device ในแต่ละ Plant ที่มอบหมายไว้ในการ์ด Plants ด้านบน"
    >
      <DataTable
        value={connections}
        dataKey="connectionId"
        aria-label="Connections"
        paginator={connections.length > 10}
        rows={10}
        emptyMessage={<div className="table-state">ยังไม่มี Device ที่ตั้งค่า IP/Port ใน Plant ที่มอบหมายไว้</div>}
      >
        <TableColumn header="Connection" body={(row: MiddlewareConnection) => (
          <div className="grid min-w-0 gap-1"><strong className="truncate text-ink">{row.connectionName}</strong><small className="truncate text-[11px] text-ink-soft">{row.devDn || "-"}</small></div>
        )} />
        <TableColumn header="Host:Port" body={(row: MiddlewareConnection) => `${row.host}:${row.port}`} />
        <TableColumn header="Device Set" body={(row: MiddlewareConnection) => snapshot?.deviceSets.find((ds) => ds.deviceSetId === row.deviceSetId)?.devModel ?? "-"} />
        <TableColumn field="plantCode" header="Plant" sortable body={(row: MiddlewareConnection) => row.plantCode || "-"} />
        <TableColumn field="enabled" header="สถานะ" sortable body={(row: MiddlewareConnection) => <StatusTag tone={row.enabled ? "active" : "revoked"}>{row.enabled ? "เปิด" : "ปิด"}</StatusTag>} />
      </DataTable>
    </Card>
  );
}

function MiddlewareConfigEditor({ gateway, onBack }: { gateway: MiddlewareGateway; onBack: () => void }) {
  const [snapshot, setSnapshot] = useState<MiddlewareConfigSnapshot | null>(null);
  const [assignedPlants, setAssignedPlants] = useState<Plant[]>([]);
  const [allPlants, setAllPlants] = useState<Plant[]>([]);
  const [addPlantId, setAddPlantId] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [importing, setImporting] = useState(false);
  const [pushing, setPushing] = useState(false);
  const [patches, setPatches] = useState<MiddlewarePatch[]>([]);
  const [selectedPatchId, setSelectedPatchId] = useState("");
  const [uploading, setUploading] = useState(false);
  const [staging, setStaging] = useState(false);
  const [applying, setApplying] = useState(false);
  // Tracks the freshest copy of this gateway's own list-row fields (online
  // state, config version, software version) so the status header above the
  // cards reflects reality after a push/apply instead of the stale snapshot
  // captured at the moment the operator clicked into this page.
  const [liveGateway, setLiveGateway] = useState<MiddlewareGateway>(gateway);
  const [rollingBack, setRollingBack] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const lifecycleBusy = uploading || staging || applying || rollingBack || restarting;

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [configResponse, plantsResponse, allPlantsResponse, patchesResponse, gatewaysResponse] = await Promise.all([
        api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/config`),
        api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/plants`),
        api("/api/v1/plants"),
        api("/api/v1/admin/middleware-patches"),
        api("/api/v1/admin/middlewares"),
      ]);
      if (!configResponse.ok || !plantsResponse.ok || !allPlantsResponse.ok) throw new Error("ไม่สามารถโหลดข้อมูล Middleware ได้");
      setSnapshot((await configResponse.json()) as MiddlewareConfigSnapshot);
      setAssignedPlants((await plantsResponse.json()) as Plant[]);
      setAllPlants((await allPlantsResponse.json()) as Plant[]);
      if (patchesResponse.ok) setPatches((await patchesResponse.json()) as MiddlewarePatch[]);
      if (gatewaysResponse.ok) {
        const current = ((await gatewaysResponse.json()) as MiddlewareGateway[]).find((item) => item.id === gateway.id);
        if (current) setLiveGateway(current);
      }
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, [gateway.id]);

  useEffect(() => { void load(); }, [load]);

  const unassignedPlants = allPlants.filter((p) => !assignedPlants.some((a) => a.id === p.id));

  async function waitForSoftwareVersion(target: string) {
    const deadline = Date.now() + 30000;
    while (Date.now() < deadline) {
      await new Promise((resolve) => setTimeout(resolve, 2000));
      const response = await api("/api/v1/admin/middlewares");
      if (!response.ok) continue;
      const current = ((await response.json()) as MiddlewareGateway[]).find((item) => item.id === gateway.id);
      if (current) setLiveGateway(current);
      if (current?.softwareVersion === target) return true;
    }
    return false;
  }

  async function assignPlant() {
    if (!addPlantId) return;
    setPending(true);
    setError("");
    try {
      const response = await api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/plants`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify({ plantId: addPlantId }),
      });
      if (!response.ok) throw new Error("ไม่สามารถมอบหมาย Plant ได้ (Plant นี้อาจถูกมอบหมายให้ Middleware อื่นแล้ว)");
      toast.success(`มอบหมาย Plant ให้ ${gateway.name} แล้ว`);
      setAddPlantId("");
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPending(false);
    }
  }

  async function unassignPlant(plant: Plant) {
    if (!window.confirm(`เอา Plant "${plant.name}" ออกจาก Middleware นี้?`)) return;
    setPending(true);
    setError("");
    try {
      const response = await api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/plants/${encodeURIComponent(plant.id)}`, {
        method: "DELETE",
        headers: { "X-CSRF-Token": csrfToken() },
      });
      if (!response.ok) throw new Error("ไม่สามารถเอา Plant ออกได้");
      toast.success(`เอา "${plant.name}" ออกจาก ${gateway.name} แล้ว`);
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPending(false);
    }
  }

  async function importConfig() {
    if (!window.confirm(`ดึง Config จาก "${gateway.name}" มาสร้าง/อัปเดต Device Model, Register Metadata และ Device (IP/Port/Unit ID) ของ Plant ที่มีอยู่แล้ว?`)) return;
    setImporting(true);
    setError("");
    try {
      const response = await api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/import-config`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
      });
      if (response.status === 504) throw new Error("Middleware ไม่ตอบสนองภายในเวลาที่กำหนด");
      if (!response.ok) throw new Error("ไม่สามารถดึง Config จาก Middleware ได้");
      const result = (await response.json()) as ImportMiddlewareConfigResult;
      toast.success(
        `Import สำเร็จ: ${result.deviceModelsCreated} Model ใหม่, ${result.deviceModelsReused} Model เดิม, ${result.registerMetadataUpserted} Register, ${result.devicesCreated} Device ใหม่, ${result.devicesUpdated} Device อัปเดต IP/Port`
        + `${result.registerMetadataSkipped > 0 ? `, ข้าม ${result.registerMetadataSkipped} Register ที่ import ไม่ได้` : ""}`
        + `${result.registerMetadataPruned > 0 ? `, ลบ ${result.registerMetadataPruned} Register เก่าที่ไม่มีใน Middleware แล้ว` : ""}`
        + `${result.devicesSkipped > 0 ? `, ข้าม ${result.devicesSkipped} Connection (Plant ยังไม่มีในระบบ หรือไม่มี external_id)` : ""}`,
      );
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setImporting(false);
    }
  }

  async function pushConfig() {
    if (!window.confirm(`ส่ง Config ที่คำนวณจาก Device ปัจจุบันไปที่ "${gateway.name}"? การตั้งค่าเดิมบน Middleware จะถูกเขียนทับ`)) return;
    setPushing(true);
    setError("");
    try {
      const response = await api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/push-config`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
      });
      if (!response.ok) throw new Error("ไม่สามารถส่ง Config ไปที่ Middleware ได้");
      const result = (await response.json()) as { plantCount: number; deviceCount: number; deviceSetCount: number; delivered: boolean };
      if (result.deviceCount === 0) {
        toast.error(
          result.plantCount === 0
            ? "ไม่มี Plant ที่ assign ให้ Middleware นี้ — ไปที่การ์ด Plants ก่อน"
            : "Assign Plant แล้วแต่ไม่มี Device พร้อม push — ตรวจว่า Device ตั้ง Modbus host/port ครบ และ Device Model มี Register Metadata ที่กรอก Modbus Function Code/Register แล้ว",
        );
      } else if (result.delivered) {
        toast.success(`ส่ง Config ไปที่ ${gateway.name} แล้ว (${result.deviceCount} device, ${result.deviceSetCount} device set)`);
      } else {
        toast.error(`คำนวณ Config ไว้แล้ว (${result.deviceCount} device) แต่ยังไม่ถึง ${gateway.name} — Middleware ไม่ได้เชื่อมต่ออยู่จริงตอนนี้ กด "ส่ง Config" อีกครั้งหลังเชื่อมต่อ`);
      }
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPushing(false);
    }
  }

  async function uploadPatch(file: File) {
    setUploading(true);
    setError("");
    try {
      const form = new FormData();
      form.append("patch", file);
      const response = await api("/api/v1/admin/middleware-patches", {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: form,
      });
      if (response.status === 403) throw new Error("เฉพาะ Platform Admin เท่านั้นที่ upload patch ได้");
      if (response.status === 409) throw new Error("มี patch สำหรับ version/os/arch นี้อยู่แล้ว");
      if (!response.ok) throw new Error("Upload patch ไม่สำเร็จ (ตรวจสอบ manifest/signature ในไฟล์)");
      toast.success("Upload patch สำเร็จ");
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setUploading(false);
    }
  }

  async function deletePatch() {
    const patch = patches.find((item) => item.id === selectedPatchId);
    if (!patch || !window.confirm(`ลบ Patch ${patch.version} (${patch.os}/${patch.arch}) หรือไม่?`)) return;
    setError("");
    try {
      const response = await api(`/api/v1/admin/middleware-patches/${encodeURIComponent(patch.id)}`, {
        method: "DELETE",
        headers: { "X-CSRF-Token": csrfToken() },
      });
      if (!response.ok) throw new Error(response.status === 403 ? "เฉพาะ Platform Admin เท่านั้นที่ลบ patch ได้" : await middlewareLifecycleError(response, "ลบ patch ไม่สำเร็จ"));
      setSelectedPatchId("");
      toast.success(`ลบ Patch ${patch.version} แล้ว`);
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    }
  }

  async function stageUpdate() {
    if (!selectedPatchId) return;
    setStaging(true);
    setError("");
    try {
      const response = await api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/update/stage`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify({ patchId: selectedPatchId }),
      });
      if (!response.ok) throw new Error(await middlewareLifecycleError(response, "Middleware อาจ offline หรือไม่ตอบสนอง"));
      toast.success(`Stage patch บน ${gateway.name} แล้ว — กด "Apply" เพื่อติดตั้งจริง`);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setStaging(false);
    }
  }

  async function applyUpdate() {
    if (!window.confirm(`Apply patch ที่ stage ไว้บน "${gateway.name}"? Service จะ restart`)) return;
    setApplying(true);
    setError("");
    try {
      const targetVersion = patches.find((patch) => patch.id === selectedPatchId)?.version;
      const response = await api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/update/apply`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
      });
      if (!response.ok) throw new Error(await middlewareLifecycleError(response, "ตรวจสอบว่า stage patch ไว้แล้วหรือยัง"));
      if (targetVersion && await waitForSoftwareVersion(targetVersion)) {
        toast.success(`อัปเดต Middleware เป็น ${targetVersion} สำเร็จ`);
      } else {
        toast.error(`คำสั่ง Apply ถูกส่งแล้ว แต่ยังยืนยัน version ${targetVersion || "ใหม่"} ไม่ได้ — ตรวจสอบ service และ last-result.txt ที่เครื่อง Middleware`);
      }
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setApplying(false);
    }
  }

  async function rollbackUpdate() {
    if (!window.confirm(`Rollback "${gateway.name}" กลับไป version ก่อนหน้า? Service จะ restart`)) return;
    setRollingBack(true);
    setError("");
    try {
      const response = await api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/update/rollback`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
      });
      if (!response.ok) throw new Error(await middlewareLifecycleError(response, "อาจไม่มี backup"));
      toast.success("Rollback เริ่มแล้ว — Middleware จะ offline ชั่วครู่ระหว่าง restart");
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setRollingBack(false);
    }
  }

  async function restartMiddlewareService() {
    if (!window.confirm(`Restart service ของ "${gateway.name}"? ไม่เปลี่ยน binary แค่ restart`)) return;
    setRestarting(true);
    setError("");
    try {
      const response = await api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/restart`, {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
      });
      if (!response.ok) throw new Error(await middlewareLifecycleError(response, "Middleware อาจไม่ได้รันเป็น Service"));
      toast.success("Restart เริ่มแล้ว");
      await load();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setRestarting(false);
    }
  }

  return (
    <div className="content">
      <div className="section-heading">
        <div className="registry-title">
          <Button variant="icon" onClick={onBack} title="กลับไป Middleware" aria-label="กลับไป Middleware"><ArrowLeft size={18} /></Button>
          <div><p>{gateway.siteName || gateway.name}</p><h2>{gateway.name}</h2></div>
        </div>
        <div className="row-actions">
          <Button variant="icon" onClick={() => void load()} title="รีเฟรช" aria-label="รีเฟรช"><RefreshCw size={18} /></Button>
        </div>
      </div>
      {error && <FormMessage>{error}</FormMessage>}
      {loading && <ProgressBar label="กำลังโหลดข้อมูล Middleware..." />}
      {loading && <div className="table-state">กำลังโหลดข้อมูล</div>}

      <div className="mb-4 flex flex-wrap items-center gap-x-5 gap-y-2 rounded-[var(--radius-md)] border border-line bg-surface px-4 py-3 text-sm">
        <StatusTag tone={liveGateway.isOnline ? "active" : "revoked"}>{liveGateway.isOnline ? "Online" : "Offline"}</StatusTag>
        <span className="text-ink-soft">Config <strong className="text-ink">v{liveGateway.configAppliedVersion}</strong> / <strong className="text-ink">v{liveGateway.configVersion}</strong></span>
        <StatusTag tone={liveGateway.configAppliedVersion < liveGateway.configVersion ? "revoked" : "active"}>
          {liveGateway.configAppliedVersion < liveGateway.configVersion ? "รอ push" : "อัปเดตล่าสุดแล้ว"}
        </StatusTag>
        <span className="text-ink-soft">Software <strong className="text-ink">{liveGateway.softwareVersion || "ไม่ทราบ"}</strong></span>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <PlantsCard
          assignedPlants={assignedPlants}
          unassignedPlants={unassignedPlants}
          addPlantId={addPlantId}
          setAddPlantId={setAddPlantId}
          pending={pending}
          loading={loading}
          onAssign={() => void assignPlant()}
          onUnassign={(plant) => void unassignPlant(plant)}
        />
        <ConfigCard
          isOnline={liveGateway.isOnline}
          pushing={pushing}
          importing={importing}
          onPush={() => void pushConfig()}
          onImport={() => void importConfig()}
        />
        <SoftwareCard
          softwareVersion={liveGateway.softwareVersion}
          patches={patches}
          selectedPatchId={selectedPatchId}
          setSelectedPatchId={setSelectedPatchId}
          lifecycleBusy={lifecycleBusy}
          uploading={uploading}
          staging={staging}
          applying={applying}
          rollingBack={rollingBack}
          restarting={restarting}
          onUpload={(file) => void uploadPatch(file)}
          onStage={() => void stageUpdate()}
          onApply={() => void applyUpdate()}
          onRollback={() => void rollbackUpdate()}
          onDeletePatch={() => void deletePatch()}
          onRestart={() => void restartMiddlewareService()}
        />
        <div className="lg:col-span-2">
          <ConnectionsCard snapshot={snapshot} />
        </div>
      </div>
    </div>
  );
}
