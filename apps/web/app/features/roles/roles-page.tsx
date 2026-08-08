"use client";

import { Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { Checkbox, FormMessage, StatusTag, TextInput } from "../../components/ui/form";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, errorMessage, csrfToken } from "../../lib/api";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";
import { pagePermissionGroups, type PagePermissionGroup } from "../../lib/navigation";
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
      setError(errorMessage(cause));
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
          <Button variant="icon" onClick={() => void loadRoles()} title="รีเฟรช" aria-label="รีเฟรชรายการ Role"><RefreshCw size={18} /></Button>
          <Button compact onClick={() => setEditor("create")}><Plus size={18} /> เพิ่ม Role</Button>
        </div>
      </div>
      {error && <FormMessage>{error}</FormMessage>}
      <div className="role-table" role="table" aria-label="Role">
        <div className="role-row role-head" role="row">
          <span>Role</span><span>ขอบเขต</span><span>ประเภท</span><span aria-label="คำสั่ง" />
        </div>
        {!loading && roles.map((role) => (
          <div className="role-row" role="row" key={role.id}>
            <div><strong>{role.name}</strong><small>{role.description || "ไม่มีคำอธิบาย"}</small></div>
            <span>{role.organizationId ? "เฉพาะองค์กร" : "ทั้งระบบ"}</span>
            <StatusTag tone={role.isSystem ? "revoked" : "active"}>{role.isSystem ? "System" : "Custom"}</StatusTag>
            <div className="row-actions">
              <Button variant="icon" onClick={() => setEditor(role)} disabled={role.isSystem} title={role.isSystem ? "System role แก้ไขไม่ได้" : "แก้ไข Role"} aria-label={`แก้ไข ${role.name}`}><Pencil size={17} /></Button>
              <Button variant="icon" danger onClick={() => void deleteRole(role)} disabled={role.isSystem} title={role.isSystem ? "System role ลบไม่ได้" : "ลบ Role"} aria-label={`ลบ ${role.name}`}><Trash2 size={17} /></Button>
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

// Every permission a page's entries reference, whether or not the caller has
// loaded that page's action list yet — used both to render the sub-checklist
// and to know what to strip when a menu gets unchecked.
function permissionsForGroup(group: PagePermissionGroup, permissions: Permission[]): Permission[] {
  return permissions.filter((permission) =>
    group.entries.some((entry) => entry.resourceType === permission.resourceType && (!entry.actions || entry.actions.includes(permission.action))));
}

function RoleEditor({ role, permissions, defaultOrganizationId, onClose, onSaved }: { role?: Role; permissions: Permission[]; defaultOrganizationId?: string; onClose: () => void; onSaved: () => void }) {
  const [loadingDetail, setLoadingDetail] = useState(Boolean(role));
  const [global, setGlobal] = useState(false);
  const [name, setName] = useState(role?.name ?? "");
  const [description, setDescription] = useState(role?.description ?? "");
  const [permissionIds, setPermissionIds] = useState<string[]>([]);
  const [expandedPages, setExpandedPages] = useState<Set<string>>(new Set());
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!role) return;
    void (async () => {
      const response = await api(`/api/v1/admin/roles/${encodeURIComponent(role.id)}`);
      if (response.ok) {
        const loaded = (await response.json()) as RoleDetail;
        setName(loaded.name);
        setDescription(loaded.description);
        setPermissionIds(loaded.permissionIds);
        setGlobal(loaded.organizationId === null);
        setExpandedPages(new Set(
          pagePermissionGroups
            .filter((group) => permissionsForGroup(group, permissions).some((permission) => loaded.permissionIds.includes(permission.id)))
            .map((group) => group.href),
        ));
      } else {
        setError("ไม่สามารถโหลดรายละเอียด Role ได้");
      }
      setLoadingDetail(false);
    })();
  }, [role, permissions]);

  function togglePermission(id: string) {
    setPermissionIds((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]);
  }

  function togglePage(group: PagePermissionGroup) {
    const groupPermissionIds = permissionsForGroup(group, permissions).map((permission) => permission.id);
    setExpandedPages((current) => {
      const next = new Set(current);
      if (next.has(group.href)) {
        next.delete(group.href);
        setPermissionIds((ids) => ids.filter((id) => !groupPermissionIds.includes(id)));
      } else {
        next.add(group.href);
      }
      return next;
    });
  }

  const mappedResourceTypes = new Set(pagePermissionGroups.flatMap((group) => group.entries.map((entry) => entry.resourceType)));
  const otherGroups = groupByResourceType(permissions.filter((permission) => !mappedResourceTypes.has(permission.resourceType)));

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
      setError(errorMessage(cause));
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
              <label className="full-field">ชื่อ Role<TextInput autoFocus value={name} onChange={(event) => setName(event.target.value)} maxLength={100} required /></label>
              <label className="full-field">คำอธิบาย<TextInput value={description} onChange={(event) => setDescription(event.target.value)} maxLength={500} /></label>
              {!role && (
                <label className="toggle-field full-field">
                  <Checkbox checked={global} onChange={setGlobal} />
                  <span>Role ทั้งระบบ (ทุกองค์กรใช้ได้ ต้องมีสิทธิ์ระดับ Platform)</span>
                </label>
              )}
              <fieldset className="full-field permission-picker">
                <legend>สิทธิ์การใช้งาน</legend>
                <p className="permission-picker-hint">1) ติ๊กเมนูที่ role นี้เข้าถึงได้ 2) ในแต่ละเมนูที่ติ๊ก เลือกว่าทำ action อะไรได้บ้าง</p>
                {pagePermissionGroups.map((group) => {
                  const groupPermissions = permissionsForGroup(group, permissions);
                  if (groupPermissions.length === 0) return null;
                  const expanded = expandedPages.has(group.href);
                  const multiEntry = group.entries.length > 1;
                  return (
                    <div key={group.href} className="permission-group">
                      <label className="toggle-field permission-page-toggle">
                        <Checkbox checked={expanded} onChange={() => togglePage(group)} />
                        <strong>{group.label}</strong>
                      </label>
                      {expanded && (
                        <div className="permission-actions">
                          {multiEntry ? (
                            group.entries.map((entry) => {
                              const entryPermissions = groupPermissions.filter((permission) => permission.resourceType === entry.resourceType);
                              if (entryPermissions.length === 0) return null;
                              return (
                                <div key={entry.resourceType} className="permission-subgroup">
                                  <small>{entry.resourceType}</small>
                                  {entryPermissions.map((permission) => (
                                    <label key={permission.id} className="toggle-field">
                                      <Checkbox checked={permissionIds.includes(permission.id)} onChange={() => togglePermission(permission.id)} />
                                      <span>{permission.action}{permission.description ? ` — ${permission.description}` : ""}</span>
                                    </label>
                                  ))}
                                </div>
                              );
                            })
                          ) : (
                            groupPermissions.map((permission) => (
                              <label key={permission.id} className="toggle-field">
                                <Checkbox checked={permissionIds.includes(permission.id)} onChange={() => togglePermission(permission.id)} />
                                <span>{permission.action}{permission.description ? ` — ${permission.description}` : ""}</span>
                              </label>
                            ))
                          )}
                        </div>
                      )}
                    </div>
                  );
                })}
                {Object.entries(otherGroups).length > 0 && (
                  <div className="permission-group">
                    <strong>อื่นๆ (ไม่ผูกกับเมนูใดเมนูหนึ่ง)</strong>
                    {Object.entries(otherGroups).map(([resourceType, items]) => (
                      <div key={resourceType} className="permission-subgroup">
                        <small>{resourceType}</small>
                        {items.map((permission) => (
                          <label key={permission.id} className="toggle-field">
                            <Checkbox checked={permissionIds.includes(permission.id)} onChange={() => togglePermission(permission.id)} />
                            <span>{permission.action}{permission.description ? ` — ${permission.description}` : ""}</span>
                          </label>
                        ))}
                      </div>
                    ))}
                  </div>
                )}
              </fieldset>
              {error && <FormMessage className="full-field">{error}</FormMessage>}
              <div className="editor-actions full-field"><Button type="button" variant="secondary" onClick={onClose} disabled={pending}>ยกเลิก</Button><Button disabled={pending}>{pending ? "กำลังบันทึก" : "บันทึก"}</Button></div>
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
