"use client";

import { ArchiveX, ArrowDownToLine, ArrowLeft, ArrowUpToLine, CheckCircle2, FileUp, Loader2, Pencil, Plus, RefreshCw, RotateCcw, Save, Settings2, Trash2, X } from "lucide-react";
import { Checkbox, FormMessage, StatusTag, TextInput } from "../../components/ui/form";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, errorMessage, csrfToken } from "../../lib/api";
import type { CreatedMiddlewareGateway, ImportMiddlewareConfigResult, MiddlewareConfigSnapshot, MiddlewareGateway, MiddlewarePatch, Plant } from "../../lib/types";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "../../components/ui/tabs";
import { Tooltip, TooltipTrigger, TooltipContent } from "../../components/ui/tooltip";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";

export function MiddlewaresPage({ defaultOrganizationId }: { defaultOrganizationId?: string }) {
  const [gateways, setGateways] = useState<MiddlewareGateway[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<MiddlewareGateway | "create" | null>(null);
  const [createdKey, setCreatedKey] = useState<CreatedMiddlewareGateway | null>(null);
  const [selected, setSelected] = useState<MiddlewareGateway | null>(null);

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
    <div className="content api-keys-content">
      <div className="section-heading">
        <div><p>Site gateways</p><h2>Middleware Gateways</h2></div>
        <div className="heading-actions">
          <Button variant="icon" onClick={() => void loadGateways()} title="รีเฟรช" aria-label="รีเฟรช Middleware"><RefreshCw size={18} /></Button>
          <Button compact onClick={() => setEditor("create")}><Plus size={18} /> เพิ่ม Middleware</Button>
        </div>
      </div>
      {createdKey && (
        <section className="secret-panel">
          <div><strong>API key ใหม่สำหรับ {createdKey.name}</strong><small>ระบบจะแสดง key เต็มเฉพาะครั้งนี้ ใช้ตั้งค่าที่ site ด้วย -api-key flag หรือ gateway-config</small></div>
          <code>{createdKey.apiKey}</code>
          <Button variant="secondary" compact onClick={() => void navigator.clipboard.writeText(createdKey.apiKey)}>คัดลอก</Button>
          <Button variant="icon" onClick={() => setCreatedKey(null)} title="ปิด" aria-label="ปิดข้อความ API key"><X size={17} /></Button>
        </section>
      )}
      {error && <FormMessage>{error}</FormMessage>}
      <div className="api-key-table middleware-table" role="table" aria-label="Middleware Gateways">
        <div className="api-key-row api-key-head" role="row">
          <span>Gateway</span><span>Site</span><span>Key</span><span>Auto onboard</span><span>เชื่อมต่อ</span><span>Config version</span><span>สถานะ</span><span aria-label="คำสั่ง" />
        </div>
        {!loading && gateways.map((gateway) => (
          <div className="api-key-row" role="row" key={gateway.id}>
            <div><strong>{gateway.name}</strong><small>{gateway.id}</small></div>
            <div><span>{gateway.siteName || "-"}</span><small>{gateway.organizationName}</small></div>
            <div><span>{gateway.keyPrefix}...</span><small>ไม่แสดง secret หลังสร้าง</small></div>
            <StatusTag tone={gateway.autoOnboard ? "active" : "revoked"}>{gateway.autoOnboard ? "เปิด" : "ปิด"}</StatusTag>
            <StatusTag tone={gateway.isOnline ? "active" : "revoked"}>{gateway.isOnline ? "Online" : "Offline"}</StatusTag>
            <div><span>v{gateway.configAppliedVersion} / v{gateway.configVersion}</span><small>{gateway.configAppliedVersion < gateway.configVersion ? "รอ push ไป gateway" : "อัปเดตล่าสุดแล้ว"}</small></div>
            <StatusTag tone={gateway.isActive ? "active" : "revoked"}>{gateway.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</StatusTag>
            <div className="row-actions">
              <Button variant="icon" onClick={() => setSelected(gateway)} title="ตั้งค่า Config" aria-label={`ตั้งค่า Config ของ ${gateway.name}`}><Settings2 size={17} /></Button>
              <Button variant="icon" onClick={() => setEditor(gateway)} title="แก้ไข" aria-label={`แก้ไข ${gateway.name}`}><Pencil size={17} /></Button>
              <Button variant="icon" onClick={() => void setGatewayActive(gateway, !gateway.isActive)} title={gateway.isActive ? "ปิดใช้งาน" : "เปิดใช้งาน"} aria-label={gateway.isActive ? `ปิดใช้งาน ${gateway.name}` : `เปิดใช้งาน ${gateway.name}`}>
                {gateway.isActive ? <ArchiveX size={17} /> : <CheckCircle2 size={17} />}
              </Button>
              <Button variant="icon" danger onClick={() => void hardDeleteGateway(gateway)} title="ลบถาวร (System Admin)" aria-label={`ลบ ${gateway.name} ถาวร`}><Trash2 size={17} /></Button>
            </div>
          </div>
        ))}
        {loading && <div className="table-state">กำลังโหลด Middleware</div>}
        {!loading && !error && gateways.length === 0 && <div className="table-state">ยังไม่มี Middleware Gateway</div>}
      </div>
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
      const response = await api(gateway ? `/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}` : "/api/v1/admin/middlewares", {
        method: gateway ? "PUT" : "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify(
          gateway
            ? { name, siteName, autoOnboard, isActive, pollIntervalSeconds, apiPollingEnabled }
            : { organizationId, name, siteName, autoOnboard, pollIntervalSeconds, apiPollingEnabled },
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
  const [softwareVersion, setSoftwareVersion] = useState<string | null | undefined>(gateway.softwareVersion);
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
        setSoftwareVersion(current?.softwareVersion);
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
      setSoftwareVersion(current?.softwareVersion);
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
            ? "ไม่มี Plant ที่ assign ให้ Middleware นี้ — ไปที่แท็บ Plants ก่อน"
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
    <div className="content api-keys-content">
      <div className="section-heading">
        <div className="registry-title">
          <Button variant="icon" onClick={onBack} title="กลับไป Middleware" aria-label="กลับไป Middleware"><ArrowLeft size={18} /></Button>
          <div><p>{gateway.siteName || gateway.name}</p><h2>{gateway.name}</h2></div>
        </div>
        <div className="row-actions">
          <StatusTag tone={gateway.isOnline ? "active" : "revoked"}>{gateway.isOnline ? "Online" : "Offline"}</StatusTag>
          <Button variant="icon" onClick={() => void load()} title="รีเฟรช" aria-label="รีเฟรช"><RefreshCw size={18} /></Button>
        </div>
      </div>
      {error && <FormMessage>{error}</FormMessage>}
      {loading && <div className="table-state">กำลังโหลดข้อมูล</div>}

      <Tabs defaultValue="plants">
        <TabsList>
          <TabsTrigger value="plants">Plants</TabsTrigger>
          <TabsTrigger value="config">Push / Pull Config</TabsTrigger>
          <TabsTrigger value="update">Software Update</TabsTrigger>
          <TabsTrigger value="connections">Connections</TabsTrigger>
        </TabsList>

        <TabsContent value="plants">
          <div className="section-heading">
            <div><h3>Plants ที่ Middleware นี้ดูแล</h3><p>เลือก Device ที่ต้อง poll ผ่านหน้า Plants → Devices ของแต่ละ Plant — config ที่ส่งไป Middleware คำนวณอัตโนมัติจาก Device ที่ตั้งค่า IP/Port ไว้แล้ว</p></div>
          </div>
          <div className="api-key-table" role="table" aria-label="Assigned Plants">
            <div className="api-key-row api-key-head" role="row"><span>Plant</span><span>Timezone</span><span aria-label="คำสั่ง" /></div>
            {assignedPlants.map((plant) => (
              <div className="api-key-row" role="row" key={plant.id}>
                <div><strong>{plant.name}</strong><small>{plant.code}</small></div>
                <span>{plant.timezone}</span>
                <div className="row-actions"><Button variant="icon" danger disabled={pending} onClick={() => void unassignPlant(plant)} title="เอาออก" aria-label={`เอา ${plant.name} ออก`}><Trash2 size={17} /></Button></div>
              </div>
            ))}
            {!loading && assignedPlants.length === 0 && <div className="table-state">ยังไม่ได้มอบหมาย Plant ให้ Middleware นี้</div>}
          </div>
          <div className="row-actions" style={{ marginTop: 12 }}>
            <Select value={addPlantId} onValueChange={setAddPlantId} disabled={unassignedPlants.length === 0}>
              <SelectTrigger className="w-64"><SelectValue placeholder="เลือก Plant ที่จะมอบหมาย..." /></SelectTrigger>
              <SelectContent>{unassignedPlants.map((p) => <SelectItem key={p.id} value={p.id}>{p.code} - {p.name}</SelectItem>)}</SelectContent>
            </Select>
            <Button compact disabled={!addPlantId || pending} onClick={() => void assignPlant()}><Plus size={16} /> มอบหมาย Plant</Button>
          </div>
        </TabsContent>

        <TabsContent value="config">
          <div className="section-heading">
            <div><h3>Push / Pull Config (Manual)</h3><p>Config ไม่ถูกส่งอัตโนมัติอีกต่อไป — เลือกทิศทางเอง: "ส่ง Config" เขียนทับค่าบน Middleware ด้วยค่าจาก ygate, "ดึง Config" อ่านค่าเดิมจาก Middleware เข้ามาไว้ใน ygate (ใช้ตอน onboard Middleware เก่า)</p></div>
            <div className="heading-actions">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="secondary" compact iconOnly disabled={pushing || importing} onClick={() => void pushConfig()} aria-label="ส่ง Config ไปที่ Middleware">
                    {pushing ? <Loader2 size={17} className="animate-spin" /> : <ArrowUpToLine size={17} />}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{pushing ? "กำลังส่ง Config..." : "ส่ง Config ไปที่ Middleware (เขียนทับค่าบน Middleware)"}</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="secondary" compact iconOnly disabled={!gateway.isOnline || importing || pushing} onClick={() => void importConfig()} aria-label="ดึง Config จาก Middleware">
                    {importing ? <Loader2 size={17} className="animate-spin" /> : <ArrowDownToLine size={17} />}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{!gateway.isOnline ? "Middleware ต้อง Online ก่อน" : importing ? "กำลังดึง Config..." : "ดึง Config จาก Middleware"}</TooltipContent>
              </Tooltip>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="update">
          <div className="section-heading">
            <div>
              <h3>Software Update (Platform Admin)</h3>
              <p>Version ปัจจุบัน: {softwareVersion || "ไม่ทราบ (Middleware ยังไม่เคยส่ง version มา)"} — upload patch, stage ไปที่เครื่องนี้, แล้วค่อย Apply แยกกัน (ไม่ auto)</p>
            </div>
            <div className="heading-actions">
              <Tooltip>
                <TooltipTrigger asChild>
                  <label
                    className={`inline-flex items-center justify-center gap-1.5 rounded-[var(--radius-md)] border border-line bg-surface px-2.5 py-1.5 text-xs font-bold text-ink transition hover:bg-canvas focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus ${lifecycleBusy ? "pointer-events-none opacity-48" : "cursor-pointer"}`}
                    aria-label="Upload Patch (.zip)"
                  >
                    {uploading ? <Loader2 size={17} className="animate-spin" /> : <FileUp size={17} />}
                    <input
                      type="file"
                      accept=".zip"
                      disabled={lifecycleBusy}
                      style={{ display: "none" }}
                      onChange={(event) => {
                        const file = event.target.files?.[0];
                        event.target.value = "";
                        if (file) void uploadPatch(file);
                      }}
                    />
                  </label>
                </TooltipTrigger>
                <TooltipContent>{uploading ? "กำลัง Upload..." : "Upload Patch (.zip)"}</TooltipContent>
              </Tooltip>
            </div>
          </div>
          <div className="row-actions" style={{ marginTop: 12 }}>
            <Select value={selectedPatchId} onValueChange={setSelectedPatchId} disabled={patches.length === 0}>
              <SelectTrigger className="w-64"><SelectValue placeholder="เลือก Patch..." /></SelectTrigger>
              <SelectContent>
                {patches.map((p) => <SelectItem key={p.id} value={p.id}>{p.version} ({p.os}/{p.arch})</SelectItem>)}
              </SelectContent>
            </Select>
            <Button variant="secondary" compact disabled={!selectedPatchId || lifecycleBusy} onClick={() => void stageUpdate()}>
              {staging ? "กำลัง Stage..." : "Stage"}
            </Button>
            <Button variant="text" compact danger disabled={!selectedPatchId || lifecycleBusy} onClick={() => void deletePatch()}>ลบ Patch</Button>
            <Button compact disabled={lifecycleBusy} onClick={() => void applyUpdate()}>
              {applying ? "กำลัง Apply..." : "Apply"}
            </Button>
            <Button variant="text" compact disabled={lifecycleBusy} onClick={() => void rollbackUpdate()}>
              {rollingBack ? "กำลัง Rollback..." : "Rollback"}
            </Button>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="text" compact iconOnly disabled={lifecycleBusy} onClick={() => void restartMiddlewareService()} aria-label="Restart Service">
                  {restarting ? <Loader2 size={17} className="animate-spin" /> : <RotateCcw size={17} />}
                </Button>
              </TooltipTrigger>
              <TooltipContent>{restarting ? "กำลัง Restart..." : "Restart service (ไม่เปลี่ยน binary แค่ restart)"}</TooltipContent>
            </Tooltip>
          </div>
        </TabsContent>

        <TabsContent value="connections">
          <div className="section-heading"><div><h3>Config ที่คำนวณล่าสุด{snapshot ? ` (v${snapshot.version})` : ""}</h3><p>อ่านอย่างเดียว — มาจาก Device ในแต่ละ Plant ที่มอบหมายไว้ในแท็บ Plants</p></div></div>
          <div className="device-table" role="table" aria-label="Connections">
            <div className="device-row device-head" role="row"><span>Connection</span><span>Host:Port</span><span>Device Set</span><span>Plant</span><span>สถานะ</span><span aria-label="คำสั่ง" /></div>
            {snapshot?.connections.map((connection) => (
              <div className="device-row" role="row" key={connection.connectionId}>
                <div><strong>{connection.connectionName}</strong><small>{connection.devDn || "-"}</small></div>
                <span>{connection.host}:{connection.port}</span>
                <span>{snapshot.deviceSets.find((ds) => ds.deviceSetId === connection.deviceSetId)?.devModel ?? "-"}</span>
                <span>{connection.plantCode || "-"}</span>
                <StatusTag tone={connection.enabled ? "active" : "revoked"}>{connection.enabled ? "เปิด" : "ปิด"}</StatusTag>
                <span />
              </div>
            ))}
            {!loading && (!snapshot || snapshot.connections.length === 0) && <div className="table-state">ยังไม่มี Device ที่ตั้งค่า IP/Port ใน Plant ที่มอบหมายไว้</div>}
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
}
