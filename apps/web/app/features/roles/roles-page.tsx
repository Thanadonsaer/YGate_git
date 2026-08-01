"use client";

import { Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, csrfToken } from "../../lib/api";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { toast } from "../../components/ui/sonner";
import type { Permission, Role, RoleDetail } from "../../lib/types";

export function RolesPage({ defaultOrganizationId }: { defaultOrganizationId?: string }) {
  const [roles, setRoles] = useState<Role[]>([]);
  const [permissions, setPermissions] = useState<Permission[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<Role | "create" | null>(null);

  const loadRoles = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [roleResponse, permissionResponse] = await Promise.all([api("/api/v1/admin/roles"), api("/api/v1/admin/permissions")]);
      if (roleResponse.status === 403 || permissionResponse.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดูข้อมูล Role");
      if (!roleResponse.ok || !permissionResponse.ok) throw new Error("ไม่สามารถโหลดข้อมูล Role ได้");
      setRoles((await roleResponse.json()) as Role[]);
      setPermissions((await permissionResponse.json()) as Permission[]);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadRoles(); }, [loadRoles]);

  async function deleteRole(role: Role) {
    if (!window.confirm(`ลบ Role "${role.name}"? คำสั่งนี้ทำย้อนกลับไม่ได้`)) return;
    const response = await api(`/api/v1/admin/roles/${encodeURIComponent(role.id)}`, {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrfToken() },
    });
    if (response.ok) { toast.success(`ลบ Role "${role.name}" แล้ว`); await loadRoles(); return; }
    if (response.status === 409) setError(`ไม่สามารถลบ "${role.name}" ได้ เพราะยังมีผู้ใช้ถูก assign role นี้อยู่`);
    else if (response.status === 403) setError("บัญชีนี้ไม่มีสิทธิ์ลบ Role นี้");
    else setError("ไม่สามารถลบ Role ได้");
  }

  return (
    <div className="content plants-content">
      <div className="section-heading">
        <div><p>Access management</p><h2>Role และสิทธิ์การใช้งาน</h2></div>
        <div className="heading-actions">
          <button className="icon-button" onClick={() => void loadRoles()} title="รีเฟรช" aria-label="รีเฟรชรายการ Role"><RefreshCw size={18} /></button>
          <button className="primary-button compact" onClick={() => setEditor("create")}><Plus size={18} /> เพิ่ม Role</button>
        </div>
      </div>
      {error && <p className="form-message error">{error}</p>}
      <div className="role-table" role="table" aria-label="Role">
        <div className="role-row role-head" role="row">
          <span>Role</span><span>ขอบเขต</span><span>ประเภท</span><span aria-label="คำสั่ง" />
        </div>
        {!loading && roles.map((role) => (
          <div className="role-row" role="row" key={role.id}>
            <div><strong>{role.name}</strong><small>{role.description || "ไม่มีคำอธิบาย"}</small></div>
            <span>{role.organizationId ? "เฉพาะองค์กร" : "ทั้งระบบ"}</span>
            <span className={role.isSystem ? "status revoked" : "status active"}>{role.isSystem ? "System" : "Custom"}</span>
            <div className="row-actions">
              <button className="icon-button" onClick={() => setEditor(role)} disabled={role.isSystem} title={role.isSystem ? "System role แก้ไขไม่ได้" : "แก้ไข Role"} aria-label={`แก้ไข ${role.name}`}><Pencil size={17} /></button>
              <button className="icon-button danger" onClick={() => void deleteRole(role)} disabled={role.isSystem} title={role.isSystem ? "System role ลบไม่ได้" : "ลบ Role"} aria-label={`ลบ ${role.name}`}><Trash2 size={17} /></button>
            </div>
          </div>
        ))}
        {loading && <div className="table-state">กำลังโหลดข้อมูล</div>}
        {!loading && !error && roles.length === 0 && <div className="table-state">ยังไม่มี Role ในขอบเขตที่คุณเข้าถึงได้</div>}
      </div>
      {editor && (
        <RoleEditor
          role={editor === "create" ? undefined : editor}
          permissions={permissions}
          defaultOrganizationId={defaultOrganizationId}
          onClose={() => setEditor(null)}
          onSaved={() => { setEditor(null); void loadRoles(); }}
        />
      )}
    </div>
  );
}

function RoleEditor({ role, permissions, defaultOrganizationId, onClose, onSaved }: { role?: Role; permissions: Permission[]; defaultOrganizationId?: string; onClose: () => void; onSaved: () => void }) {
  const [detail, setDetail] = useState<RoleDetail | null>(null);
  const [loadingDetail, setLoadingDetail] = useState(Boolean(role));
  const [global, setGlobal] = useState(false);
  const [name, setName] = useState(role?.name ?? "");
  const [description, setDescription] = useState(role?.description ?? "");
  const [permissionIds, setPermissionIds] = useState<string[]>([]);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!role) return;
    void (async () => {
      const response = await api(`/api/v1/admin/roles/${encodeURIComponent(role.id)}`);
      if (response.ok) {
        const loaded = (await response.json()) as RoleDetail;
        setDetail(loaded);
        setName(loaded.name);
        setDescription(loaded.description);
        setPermissionIds(loaded.permissionIds);
        setGlobal(loaded.organizationId === null);
      } else {
        setError("ไม่สามารถโหลดรายละเอียด Role ได้");
      }
      setLoadingDetail(false);
    })();
  }, [role]);

  function togglePermission(id: string) {
    setPermissionIds((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]);
  }

  const groups = groupByResourceType(permissions);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const body = role
        ? { name, description, permissionIds }
        : { global, organizationId: defaultOrganizationId ?? "", name, description, permissionIds };
      const response = await api(role ? `/api/v1/admin/roles/${encodeURIComponent(role.id)}` : "/api/v1/admin/roles", {
        method: role ? "PUT" : "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify(body),
      });
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์บันทึก Role นี้");
      if (response.status === 409) throw new Error("ชื่อ Role นี้ถูกใช้งานแล้วในขอบเขตเดียวกัน");
      if (!response.ok) throw new Error("ข้อมูลไม่ถูกต้องหรือไม่สามารถบันทึกได้");
      onSaved();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open={true} onOpenChange={(open) => { if (!open && !pending) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <div>
            <DialogDescription>Access management</DialogDescription>
            <DialogTitle>{role ? "แก้ไข Role" : "เพิ่ม Role"}</DialogTitle>
          </div>
        </DialogHeader>
        <DialogBody>
          {loadingDetail ? <div className="table-state">กำลังโหลดข้อมูล</div> : (
            <form className="plant-editor-form" onSubmit={submit}>
              <label className="full-field">ชื่อ Role<input autoFocus value={name} onChange={(event) => setName(event.target.value)} maxLength={100} required /></label>
              <label className="full-field">คำอธิบาย<input value={description} onChange={(event) => setDescription(event.target.value)} maxLength={500} /></label>
              {!role && (
                <label className="toggle-field full-field">
                  <input type="checkbox" checked={global} onChange={(event) => setGlobal(event.target.checked)} />
                  <span>Role ทั้งระบบ (ทุกองค์กรใช้ได้ ต้องมีสิทธิ์ระดับ Platform)</span>
                </label>
              )}
              <fieldset className="full-field permission-picker">
                <legend>สิทธิ์การใช้งาน</legend>
                {Object.entries(groups).map(([resourceType, items]) => (
                  <div key={resourceType} className="permission-group">
                    <strong>{resourceType}</strong>
                    {items.map((permission) => (
                      <label key={permission.id} className="toggle-field">
                        <input type="checkbox" checked={permissionIds.includes(permission.id)} onChange={() => togglePermission(permission.id)} />
                        <span>{permission.action}{permission.description ? ` — ${permission.description}` : ""}</span>
                      </label>
                    ))}
                  </div>
                ))}
              </fieldset>
              {error && <p className="form-message error full-field">{error}</p>}
              <div className="editor-actions full-field"><button type="button" className="secondary-button" onClick={onClose} disabled={pending}>ยกเลิก</button><button className="primary-button" disabled={pending}>{pending ? "กำลังบันทึก" : "บันทึก"}</button></div>
            </form>
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

function groupByResourceType(permissions: Permission[]) {
  const groups: Record<string, Permission[]> = {};
  for (const permission of permissions) {
    (groups[permission.resourceType] ??= []).push(permission);
  }
  return groups;
}
