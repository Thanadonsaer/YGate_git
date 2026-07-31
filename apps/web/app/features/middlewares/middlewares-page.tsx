"use client";

import { ArchiveX, ArrowLeft, CheckCircle2, Pencil, Plus, RefreshCw, Save, Settings2, Trash2, X } from "lucide-react";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, csrfToken } from "../../lib/api";
import type { CreatedMiddlewareGateway, MiddlewareConfigSnapshot, MiddlewareGateway, Plant } from "../../lib/types";

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
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
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
      body: JSON.stringify({ name: gateway.name, siteName: gateway.siteName, isActive }),
    });
    if (response.ok) await loadGateways();
    else setError(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์เปลี่ยนสถานะ Middleware" : "ไม่สามารถเปลี่ยนสถานะ Middleware ได้");
  }

  if (selected) {
    return <MiddlewareConfigEditor gateway={selected} onBack={() => setSelected(null)} />;
  }

  return (
    <div className="content api-keys-content">
      <div className="section-heading">
        <div><p>Site gateways</p><h2>Middleware Gateways</h2></div>
        <div className="heading-actions">
          <button className="icon-button" onClick={() => void loadGateways()} title="รีเฟรช" aria-label="รีเฟรช Middleware"><RefreshCw size={18} /></button>
          <button className="primary-button compact" onClick={() => setEditor("create")}><Plus size={18} /> เพิ่ม Middleware</button>
        </div>
      </div>
      {createdKey && (
        <section className="secret-panel">
          <div><strong>API key ใหม่สำหรับ {createdKey.name}</strong><small>ระบบจะแสดง key เต็มเฉพาะครั้งนี้ ใช้ตั้งค่าที่ site ด้วย -api-key flag หรือ gateway-config</small></div>
          <code>{createdKey.apiKey}</code>
          <button className="secondary-button compact" onClick={() => void navigator.clipboard.writeText(createdKey.apiKey)}>คัดลอก</button>
          <button className="icon-button" onClick={() => setCreatedKey(null)} title="ปิด" aria-label="ปิดข้อความ API key"><X size={17} /></button>
        </section>
      )}
      {error && <p className="form-message error">{error}</p>}
      <div className="api-key-table" role="table" aria-label="Middleware Gateways">
        <div className="api-key-row api-key-head" role="row">
          <span>Gateway</span><span>Site</span><span>Key</span><span>เชื่อมต่อ</span><span>Config version</span><span>สถานะ</span><span aria-label="คำสั่ง" />
        </div>
        {!loading && gateways.map((gateway) => (
          <div className="api-key-row" role="row" key={gateway.id}>
            <div><strong>{gateway.name}</strong><small>{gateway.id}</small></div>
            <div><span>{gateway.siteName || "-"}</span><small>{gateway.organizationName}</small></div>
            <div><span>{gateway.keyPrefix}...</span><small>ไม่แสดง secret หลังสร้าง</small></div>
            <span className={gateway.isOnline ? "status active" : "status revoked"}>{gateway.isOnline ? "Online" : "Offline"}</span>
            <div><span>v{gateway.configAppliedVersion} / v{gateway.configVersion}</span><small>{gateway.configAppliedVersion < gateway.configVersion ? "รอ push ไป gateway" : "อัปเดตล่าสุดแล้ว"}</small></div>
            <span className={gateway.isActive ? "status active" : "status revoked"}>{gateway.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</span>
            <div className="row-actions">
              <button className="icon-button" onClick={() => setSelected(gateway)} title="ตั้งค่า Config" aria-label={`ตั้งค่า Config ของ ${gateway.name}`}><Settings2 size={17} /></button>
              <button className="icon-button" onClick={() => setEditor(gateway)} title="แก้ไข" aria-label={`แก้ไข ${gateway.name}`}><Pencil size={17} /></button>
              <button className="icon-button" onClick={() => void setGatewayActive(gateway, !gateway.isActive)} title={gateway.isActive ? "ปิดใช้งาน" : "เปิดใช้งาน"} aria-label={gateway.isActive ? `ปิดใช้งาน ${gateway.name}` : `เปิดใช้งาน ${gateway.name}`}>
                {gateway.isActive ? <ArchiveX size={17} /> : <CheckCircle2 size={17} />}
              </button>
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
  const [isActive, setIsActive] = useState(gateway?.isActive ?? true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const response = await api(gateway ? `/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}` : "/api/v1/admin/middlewares", {
        method: gateway ? "PUT" : "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify(gateway ? { name, siteName, isActive } : { organizationId, name, siteName }),
      });
      if (!response.ok) throw new Error(response.status === 409 ? "ชื่อ Middleware นี้มีอยู่แล้ว" : response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์จัดการ Middleware" : "ไม่สามารถบันทึก Middleware ได้");
      onSaved(gateway ? undefined : ((await response.json()) as CreatedMiddlewareGateway));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (!pending && event.target === event.currentTarget) onClose(); }}>
      <section className="plant-editor api-key-editor" role="dialog" aria-modal="true" aria-labelledby="middleware-editor-title">
        <header><div><p>Middleware gateway</p><h2 id="middleware-editor-title">{gateway ? "แก้ไข Middleware" : "เพิ่ม Middleware"}</h2></div><button className="icon-button" onClick={onClose} disabled={pending} title="ปิด" aria-label="ปิด"><X size={18} /></button></header>
        <form onSubmit={submit}>
          <label className="full-field">ชื่อ Gateway<input autoFocus value={name} onChange={(event) => setName(event.target.value)} maxLength={200} required /></label>
          <label className="full-field">ชื่อ Site<input value={siteName} onChange={(event) => setSiteName(event.target.value)} maxLength={200} placeholder="เช่น VT1 - Vientiane Solar" /></label>
          {!gateway && <label className="full-field">Organization ID<input value={organizationId} onChange={(event) => setOrganizationId(event.target.value)} required /></label>}
          {gateway && <label className="toggle-field full-field"><input type="checkbox" checked={isActive} onChange={(event) => setIsActive(event.target.checked)} /> เปิดใช้งาน Middleware</label>}
          {error && <p className="form-message error full-field">{error}</p>}
          <div className="editor-actions full-field"><button type="button" className="secondary-button" onClick={onClose} disabled={pending}>ยกเลิก</button><button className="primary-button" disabled={pending}><Save size={17} /> {pending ? "กำลังบันทึก" : "บันทึก Middleware"}</button></div>
        </form>
      </section>
    </div>
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

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [configResponse, plantsResponse, allPlantsResponse] = await Promise.all([
        api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/config`),
        api(`/api/v1/admin/middlewares/${encodeURIComponent(gateway.id)}/plants`),
        api("/api/v1/plants"),
      ]);
      if (!configResponse.ok || !plantsResponse.ok || !allPlantsResponse.ok) throw new Error("ไม่สามารถโหลดข้อมูล Middleware ได้");
      setSnapshot((await configResponse.json()) as MiddlewareConfigSnapshot);
      setAssignedPlants((await plantsResponse.json()) as Plant[]);
      setAllPlants((await allPlantsResponse.json()) as Plant[]);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setLoading(false);
    }
  }, [gateway.id]);

  useEffect(() => { void load(); }, [load]);

  const unassignedPlants = allPlants.filter((p) => !assignedPlants.some((a) => a.id === p.id));

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
      setAddPlantId("");
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
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
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="content api-keys-content">
      <div className="section-heading">
        <div className="registry-title">
          <button className="icon-button" onClick={onBack} title="กลับไป Middleware" aria-label="กลับไป Middleware"><ArrowLeft size={18} /></button>
          <div><p>{gateway.siteName || gateway.name}</p><h2>{gateway.name}</h2></div>
        </div>
        <div className="row-actions">
          <span className={gateway.isOnline ? "status active" : "status revoked"}>{gateway.isOnline ? "Online" : "Offline"}</span>
          <button className="icon-button" onClick={() => void load()} title="รีเฟรช" aria-label="รีเฟรช"><RefreshCw size={18} /></button>
        </div>
      </div>
      {error && <p className="form-message error">{error}</p>}
      {loading && <div className="table-state">กำลังโหลดข้อมูล</div>}

      <div className="section-heading">
        <div><h3>Plants ที่ Middleware นี้ดูแล</h3><p>เลือก Device ที่ต้อง poll ผ่านหน้า Plants → Devices ของแต่ละ Plant — config ที่ส่งไป Middleware คำนวณอัตโนมัติจาก Device ที่ตั้งค่า IP/Port ไว้แล้ว</p></div>
      </div>
      <div className="api-key-table" role="table" aria-label="Assigned Plants">
        <div className="api-key-row api-key-head" role="row"><span>Plant</span><span>Timezone</span><span aria-label="คำสั่ง" /></div>
        {assignedPlants.map((plant) => (
          <div className="api-key-row" role="row" key={plant.id}>
            <div><strong>{plant.name}</strong><small>{plant.code}</small></div>
            <span>{plant.timezone}</span>
            <div className="row-actions"><button className="icon-button danger" disabled={pending} onClick={() => void unassignPlant(plant)} title="เอาออก" aria-label={`เอา ${plant.name} ออก`}><Trash2 size={17} /></button></div>
          </div>
        ))}
        {!loading && assignedPlants.length === 0 && <div className="table-state">ยังไม่ได้มอบหมาย Plant ให้ Middleware นี้</div>}
      </div>
      <div className="row-actions" style={{ marginTop: 12 }}>
        <select value={addPlantId} onChange={(event) => setAddPlantId(event.target.value)}>
          <option value="">เลือก Plant ที่จะมอบหมาย...</option>
          {unassignedPlants.map((p) => <option key={p.id} value={p.id}>{p.code} - {p.name}</option>)}
        </select>
        <button className="primary-button compact" disabled={!addPlantId || pending} onClick={() => void assignPlant()}><Plus size={16} /> มอบหมาย Plant</button>
      </div>

      {snapshot && (
        <>
          <div className="section-heading"><div><h3>Config ที่คำนวณล่าสุด (v{snapshot.version})</h3><p>อ่านอย่างเดียว — มาจาก Device ในแต่ละ Plant ที่มอบหมายไว้ด้านบน</p></div></div>
          <div className="device-table" role="table" aria-label="Connections">
            <div className="device-row device-head" role="row"><span>Connection</span><span>Host:Port</span><span>Device Set</span><span>Plant</span><span>สถานะ</span><span aria-label="คำสั่ง" /></div>
            {snapshot.connections.map((connection) => (
              <div className="device-row" role="row" key={connection.connectionId}>
                <div><strong>{connection.connectionName}</strong><small>{connection.devDn || "-"}</small></div>
                <span>{connection.host}:{connection.port}</span>
                <span>{snapshot.deviceSets.find((ds) => ds.deviceSetId === connection.deviceSetId)?.devModel ?? "-"}</span>
                <span>{connection.plantCode || "-"}</span>
                <span className={connection.enabled ? "status active" : "status revoked"}>{connection.enabled ? "เปิด" : "ปิด"}</span>
                <span />
              </div>
            ))}
            {snapshot.connections.length === 0 && <div className="table-state">ยังไม่มี Device ที่ตั้งค่า IP/Port ใน Plant ที่มอบหมายไว้</div>}
          </div>
        </>
      )}
    </div>
  );
}
