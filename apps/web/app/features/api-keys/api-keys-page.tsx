"use client";

import { ArchiveX, CheckCircle2, Pencil, Plus, RefreshCw, Save, Trash2, X } from "lucide-react";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, csrfToken, formatDate } from "../../lib/api";
import type { APIKeyClient, CreatedAPIKeyClient } from "../../lib/types";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { toast } from "../../components/ui/sonner";

export function APIKeysPage({ defaultOrganizationId }: { defaultOrganizationId?: string }) {
  const [clients, setClients] = useState<APIKeyClient[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<APIKeyClient | "create" | null>(null);
  const [createdKey, setCreatedKey] = useState<CreatedAPIKeyClient | null>(null);

  const loadClients = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await api("/api/v1/admin/api-keys");
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์จัดการ API Key");
      if (!response.ok) throw new Error("ไม่สามารถโหลด API Key ได้");
      setClients((await response.json()) as APIKeyClient[]);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadClients(); }, [loadClients]);

  async function setClientActive(client: APIKeyClient, isActive: boolean) {
    if (!isActive && !window.confirm(`ปิดใช้งาน API Key "${client.name}"? Middleware ที่ใช้ key นี้จะส่งข้อมูลไม่ได้ทันที`)) return;
    const response = await api(`/api/v1/admin/api-keys/${encodeURIComponent(client.id)}/status`, {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ isActive }),
    });
    if (response.ok) { toast.success(isActive ? `เปิดใช้งาน API Key "${client.name}" แล้ว` : `ปิดใช้งาน API Key "${client.name}" แล้ว`); await loadClients(); }
    else setError(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์เปลี่ยนสถานะ API Key" : "ไม่สามารถเปลี่ยนสถานะ API Key ได้");
  }

  async function hardDeleteClient(client: APIKeyClient) {
    const expected = "DELETE";
    if (window.prompt(`คำสั่งนี้จะลบ API Key และ ingestion history ของ client นี้ถาวร\nพิมพ์ ${expected}`) !== expected) return;
    try {
      const response = await api(`/api/v1/admin/api-keys/${encodeURIComponent(client.id)}`, {
        method: "DELETE",
        headers: { "X-CSRF-Token": csrfToken(), "X-Hard-Delete-Confirm": expected },
      });
      if (response.ok) await loadClients();
      else setError(response.status === 403 ? "เฉพาะ Platform Admin เท่านั้นที่ลบ API Key ถาวรได้" : "ไม่สามารถลบ API Key ถาวรได้");
    } catch {
      setError("ไม่สามารถเชื่อมต่อ Gateway ได้");
    }
  }

  return <div className="content api-keys-content">
    <div className="section-heading">
      <div><p>Middleware access</p><h2>API Keys</h2></div>
      <div className="heading-actions">
        <button className="icon-button" onClick={() => void loadClients()} title="รีเฟรช" aria-label="รีเฟรช API Keys"><RefreshCw size={18} /></button>
        <button className="primary-button compact" onClick={() => setEditor("create")}><Plus size={18} /> เพิ่ม API Key</button>
      </div>
    </div>
    {createdKey && <section className="secret-panel">
      <div><strong>API key ใหม่สำหรับ {createdKey.name}</strong><small>ระบบจะแสดง key เต็มเฉพาะครั้งนี้</small></div>
      <code>{createdKey.apiKey}</code>
      <button className="secondary-button compact" onClick={() => void navigator.clipboard.writeText(createdKey.apiKey)}>คัดลอก</button>
      <button className="icon-button" onClick={() => setCreatedKey(null)} title="ปิด" aria-label="ปิดข้อความ API key"><X size={17} /></button>
    </section>}
    {error && <p className="form-message error">{error}</p>}
    <div className="api-key-table" role="table" aria-label="API Keys">
      <div className="api-key-row api-key-head" role="row"><span>Client</span><span>องค์กร</span><span>Key</span><span>Auto onboard</span><span>Last seen</span><span>สถานะ</span><span aria-label="คำสั่ง" /></div>
      {!loading && clients.map((client) => (
        <div className="api-key-row" role="row" key={client.id}>
          <div><strong>{client.name}</strong><small>{client.id}</small></div>
          <div><span>{client.organizationName}</span><small>{client.organizationId}</small></div>
          <div><span>{client.keyPrefix}...</span><small>ไม่แสดง secret หลังสร้าง</small></div>
          <span className={client.autoOnboard ? "status active" : "status revoked"}>{client.autoOnboard ? "เปิด" : "ปิด"}</span>
          <div><span>{client.lastSeenAt ? formatDate(client.lastSeenAt) : "ยังไม่เคยใช้"}</span><small>Updated {formatDate(client.updatedAt)}</small></div>
          <span className={client.isActive ? "status active" : "status revoked"}>{client.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</span>
          <div className="row-actions">
            <button className="icon-button" onClick={() => setEditor(client)} title="แก้ไข" aria-label={`แก้ไข ${client.name}`}><Pencil size={17} /></button>
            <button className="icon-button" onClick={() => void setClientActive(client, !client.isActive)} title={client.isActive ? "ปิดใช้งาน" : "เปิดใช้งาน"} aria-label={client.isActive ? `ปิดใช้งาน ${client.name}` : `เปิดใช้งาน ${client.name}`}>{client.isActive ? <ArchiveX size={17} /> : <CheckCircle2 size={17} />}</button>
            <button className="icon-button danger" onClick={() => void hardDeleteClient(client)} title="ลบถาวร (Platform Admin)" aria-label={`ลบ ${client.name} ถาวร`}><Trash2 size={17} /></button>
          </div>
        </div>
      ))}
      {loading && <div className="table-state">กำลังโหลด API Keys</div>}
      {!loading && !error && clients.length === 0 && <div className="table-state">ยังไม่มี Middleware API Key</div>}
    </div>
    {editor && <APIKeyEditor client={editor === "create" ? undefined : editor} defaultOrganizationId={defaultOrganizationId} onClose={() => setEditor(null)} onSaved={(created) => { setEditor(null); if (created) setCreatedKey(created); void loadClients(); }} />}
  </div>;
}

function APIKeyEditor({ client, defaultOrganizationId, onClose, onSaved }: { client?: APIKeyClient; defaultOrganizationId?: string; onClose: () => void; onSaved: (created?: CreatedAPIKeyClient) => void }) {
  const [organizationId, setOrganizationId] = useState(client?.organizationId ?? defaultOrganizationId ?? "");
  const [name, setName] = useState(client?.name ?? "");
  const [autoOnboard, setAutoOnboard] = useState(client?.autoOnboard ?? true);
  const [isActive, setIsActive] = useState(client?.isActive ?? true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const response = await api(client ? `/api/v1/admin/api-keys/${encodeURIComponent(client.id)}` : "/api/v1/admin/api-keys", {
        method: client ? "PUT" : "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify(client ? { name, autoOnboard, isActive } : { organizationId, name, autoOnboard }),
      });
      if (!response.ok) throw new Error(response.status === 409 ? "ชื่อ API Key นี้มีอยู่แล้ว" : response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์จัดการ API Key" : "ไม่สามารถบันทึก API Key ได้");
      onSaved(client ? undefined : (await response.json()) as CreatedAPIKeyClient);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !pending) onClose(); }}>
      <DialogContent>
        <DialogHeader>
          <div><DialogDescription>Middleware API key</DialogDescription><DialogTitle>{client ? "แก้ไข API Key" : "เพิ่ม API Key"}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={submit}>
            <label className="full-field">ชื่อ Client<input value={name} onChange={(event) => setName(event.target.value)} maxLength={200} required /></label>
            <label className="full-field">Organization ID<input value={organizationId} onChange={(event) => setOrganizationId(event.target.value)} required disabled={Boolean(client)} /></label>
            <label className="toggle-field full-field"><input type="checkbox" checked={autoOnboard} onChange={(event) => setAutoOnboard(event.target.checked)} /> Auto onboard Plant/Device</label>
            {client && <label className="toggle-field full-field"><input type="checkbox" checked={isActive} onChange={(event) => setIsActive(event.target.checked)} /> เปิดใช้งาน API Key</label>}
            {error && <p className="form-message error full-field">{error}</p>}
            <div className="editor-actions full-field"><button type="button" className="secondary-button" onClick={onClose} disabled={pending}>ยกเลิก</button><button className="primary-button" disabled={pending}><Save size={17} /> {pending ? "กำลังบันทึก" : "บันทึก API Key"}</button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
