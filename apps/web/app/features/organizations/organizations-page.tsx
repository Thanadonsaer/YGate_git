"use client";

import { Pencil, Plus, RefreshCw } from "lucide-react";
import { Checkbox, FormMessage, StatusTag, TextInput } from "../../components/ui/form";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, errorMessage, csrfToken, formatDate } from "../../lib/api";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";
import { DataTable, TableColumn } from "../../components/ui/data-table";
import type { Organization } from "../../lib/types";

export function OrganizationsPage() {
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<Organization | "create" | null>(null);

  const loadOrganizations = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await api("/api/v1/admin/organizations");
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดูข้อมูล Organization");
      if (!response.ok) throw new Error("ไม่สามารถโหลดข้อมูล Organization ได้");
      setOrganizations((await response.json()) as Organization[]);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadOrganizations(); }, [loadOrganizations]);

  return (
    <div className="content plants-content">
      <div className="section-heading">
        <div><p>Tenant management</p><h2>Organization ทั้งหมด</h2></div>
        <div className="heading-actions">
          <Button variant="icon" onClick={() => void loadOrganizations()} title="รีเฟรช" aria-label="รีเฟรชรายการ Organization"><RefreshCw size={18} /></Button>
          <Button compact onClick={() => setEditor("create")}><Plus size={18} /> เพิ่ม Organization</Button>
        </div>
      </div>
      {error && <FormMessage>{error}</FormMessage>}
      {loading ? (
        <div className="table-state">กำลังโหลดข้อมูล</div>
      ) : (
        <DataTable value={organizations} dataKey="id" aria-label="Organization" emptyMessage={<div className="table-state">{error ? "" : "ยังไม่มี Organization ในขอบเขตที่คุณเข้าถึงได้"}</div>}>
          <TableColumn field="name" header="Organization" sortable body={(organization: Organization) => (
            <div className="grid gap-1"><strong>{organization.name}</strong><small className="block text-[11px] text-ink-soft">{organization.id}</small></div>
          )} />
          <TableColumn field="code" header="รหัส" sortable body={(organization: Organization) => <span>{organization.code}</span>} />
          <TableColumn field="isActive" header="สถานะ" sortable body={(organization: Organization) => (
            <StatusTag tone={organization.isActive ? "active" : "revoked"}>{organization.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</StatusTag>
          )} />
          <TableColumn field="updatedAt" header="อัปเดตล่าสุด" sortable body={(organization: Organization) => <span>{formatDate(organization.updatedAt)}</span>} />
          <TableColumn header="" body={(organization: Organization) => (
            <div className="row-actions">
              <Button variant="icon" onClick={() => setEditor(organization)} title="แก้ไข Organization" aria-label={`แก้ไข ${organization.name}`}><Pencil size={17} /></Button>
            </div>
          )} />
        </DataTable>
      )}
      {editor && (
        <OrganizationEditor
          organization={editor === "create" ? undefined : editor}
          onClose={() => setEditor(null)}
          onSaved={() => { setEditor(null); void loadOrganizations(); }}
        />
      )}
    </div>
  );
}

function OrganizationEditor({ organization, onClose, onSaved }: { organization?: Organization; onClose: () => void; onSaved: () => void }) {
  const [code, setCode] = useState(organization?.code ?? "");
  const [name, setName] = useState(organization?.name ?? "");
  const [isActive, setIsActive] = useState(organization?.isActive ?? true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const response = await api(organization ? `/api/v1/admin/organizations/${encodeURIComponent(organization.id)}` : "/api/v1/admin/organizations", {
        method: organization ? "PUT" : "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify({ code, name, isActive }),
      });
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์บันทึก Organization นี้");
      if (response.status === 409) throw new Error("รหัส Organization นี้ถูกใช้งานแล้ว");
      if (!response.ok) throw new Error("ข้อมูลไม่ถูกต้องหรือไม่สามารถบันทึกได้");
      toast.success(organization ? `บันทึก "${name}" แล้ว` : `เพิ่ม "${name}" แล้ว`);
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
          <div>
            <DialogDescription>Tenant management</DialogDescription>
            <DialogTitle>{organization ? "แก้ไข Organization" : "เพิ่ม Organization"}</DialogTitle>
          </div>
        </DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={submit}>
            <label>รหัส Organization<TextInput autoFocus value={code} onChange={(event) => setCode(event.target.value)} maxLength={50} required /></label>
            <label>ชื่อ Organization<TextInput value={name} onChange={(event) => setName(event.target.value)} maxLength={200} required /></label>
            {organization && (
              <label className="toggle-field full-field">
                <Checkbox checked={isActive} onChange={setIsActive} />
                <span>เปิดใช้งาน Organization</span>
              </label>
            )}
            {error && <FormMessage className="full-field">{error}</FormMessage>}
            <div className="editor-actions full-field"><Button type="button" variant="secondary" onClick={onClose} disabled={pending}>ยกเลิก</Button><Button disabled={pending}>{pending ? "กำลังบันทึก" : "บันทึก"}</Button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
