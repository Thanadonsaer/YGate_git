"use client";

import { CheckCircle2, KeyRound, Pencil, Plus, RefreshCw, RotateCcw, Save, Trash2, UserX } from "lucide-react";
import { Checkbox, FormMessage, PasswordInput, StatusTag, TextInput } from "../../components/ui/form";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, errorMessage, csrfToken, formatDate } from "../../lib/api";
import type { ManagedUser, Organization, Role } from "../../lib/types";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "../../components/ui/select";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";
import { DataTable, TableColumn } from "../../components/ui/data-table";

export function UsersPage({ currentUserId, defaultOrganizationId }: { currentUserId: string; defaultOrganizationId?: string }) {
  const [users, setUsers] = useState<ManagedUser[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<ManagedUser | "create" | null>(null);
  const [resetTarget, setResetTarget] = useState<ManagedUser | null>(null);

  const loadUsers = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [userResponse, roleResponse, organizationResponse] = await Promise.all([api("/api/v1/admin/users"), api("/api/v1/admin/roles"), api("/api/v1/admin/organizations")]);
      if (userResponse.status === 403 || roleResponse.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์จัดการผู้ใช้");
      if (!userResponse.ok || !roleResponse.ok) throw new Error("ไม่สามารถโหลดข้อมูลผู้ใช้ได้");
      setUsers((await userResponse.json()) as ManagedUser[]);
      setRoles((await roleResponse.json()) as Role[]);
      setOrganizations(organizationResponse.ok ? ((await organizationResponse.json()) as Organization[]) : []);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void loadUsers(); }, [loadUsers]);

  async function setUserActive(target: ManagedUser, isActive: boolean) {
    if (!isActive && !window.confirm(`ปิดใช้งาน “${target.email}” และ revoke ทุก session?`)) return;
    const response = await api(`/api/v1/admin/users/${encodeURIComponent(target.id)}/status`, {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ isActive }),
    });
    if (response.ok) { toast.success(isActive ? `เปิดใช้งาน ${target.displayName} แล้ว` : `ปิดใช้งาน ${target.displayName} แล้ว`); await loadUsers(); }
    else setError(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์เปลี่ยนสถานะผู้ใช้" : response.status === 400 ? "ต้องยืนยันอีเมลและกำหนด Role ก่อนเปิดใช้งาน User" : "ไม่สามารถเปลี่ยนสถานะผู้ใช้ได้");
  }

  async function unlockUser(target: ManagedUser) {
    const response = await api(`/api/v1/admin/users/${encodeURIComponent(target.id)}/unlock`, {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
    });
    if (response.ok) { toast.success(`ปลดล็อก ${target.displayName} แล้ว`); await loadUsers(); }
    else setError(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์ปลดล็อกผู้ใช้" : "ไม่สามารถปลดล็อกผู้ใช้ได้");
  }

  async function hardDeleteUser(target: ManagedUser) {
    const expected = "DELETE";
    if (window.prompt(`ลบ User และข้อมูลที่เป็นเจ้าของแบบถาวร\nพิมพ์ ${expected} เพื่อยืนยัน`) !== expected) return;
    const response = await api(`/api/v1/admin/users/${encodeURIComponent(target.id)}`, {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrfToken(), "X-Hard-Delete-Confirm": expected },
    });
    if (response.ok) await loadUsers();
    else setError(response.status === 403 ? "เฉพาะ Platform Admin ที่มีสิทธิ์ Hard Delete" : response.status === 400 ? "ไม่สามารถลบตัวเองหรือ Platform Admin คนสุดท้ายได้" : "ไม่สามารถ Hard Delete User ได้");
  }

  const canHardDelete = users.find((item) => item.id === currentUserId)?.roles.includes("Platform Admin") ?? false;

  return <div className="content users-content">
    <div className="section-heading">
      <div><p>Access management</p><h2>ผู้ใช้ในระบบ</h2></div>
      <div className="heading-actions">
        <Button variant="icon" onClick={() => void loadUsers()} title="รีเฟรช" aria-label="รีเฟรชรายการผู้ใช้"><RefreshCw size={18} /></Button>
        <Button compact onClick={() => setEditor("create")}><Plus size={18} /> เพิ่มผู้ใช้</Button>
      </div>
    </div>
    {error && <FormMessage>{error}</FormMessage>}
    {loading ? (
      <div className="table-state">กำลังโหลดข้อมูลผู้ใช้</div>
    ) : (
      <DataTable value={users} dataKey="id" aria-label="ผู้ใช้" paginator rows={20} emptyMessage={<div className="table-state">{error ? "" : "ยังไม่มีผู้ใช้ในขอบเขตที่คุณเข้าถึงได้"}</div>}>
        <TableColumn field="displayName" header="ผู้ใช้" sortable body={(item: ManagedUser) => (
          <div className="grid gap-1"><strong>{item.displayName}</strong><small className="block text-[11px] text-ink-soft">{item.email}{item.username ? ` · ${item.username}` : ""}</small></div>
        )} />
        <TableColumn field="organizationName" header="องค์กร" sortable body={(item: ManagedUser) => (
          <div className="grid gap-1"><span>{item.organizationName}</span><small className="block text-[11px] text-ink-soft">{item.organizationId}</small></div>
        )} />
        <TableColumn header="Role" body={(item: ManagedUser) => (
          <div className="grid gap-1"><span>{item.roles.join(", ") || "-"}</span><small className="block text-[11px] text-ink-soft">Role และ profile แก้ไขได้</small></div>
        )} />
        <TableColumn field="failedLoginCount" header="Login" sortable body={(item: ManagedUser) => (
          <div className="grid gap-1"><span>{item.failedLoginCount.toLocaleString()} failed</span><small className="block text-[11px] text-ink-soft">{item.lockedUntil ? `Locked ${formatDate(item.lockedUntil)}` : `Updated ${formatDate(item.updatedAt)}`}</small></div>
        )} />
        <TableColumn field="isActive" header="สถานะ" sortable body={(item: ManagedUser) => (
          <StatusTag tone={item.isActive ? "active" : "revoked"}>{item.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</StatusTag>
        )} />
        <TableColumn header="" body={(item: ManagedUser) => (
          <div className="row-actions">
            <Button variant="icon" onClick={() => setEditor(item)} disabled={item.id === currentUserId} title="แก้ไข User/Role" aria-label={`แก้ไข ${item.displayName}`}><Pencil size={17} /></Button>
            <Button variant="icon" onClick={() => void unlockUser(item)} disabled={!item.lockedUntil && item.failedLoginCount === 0} title="ปลดล็อก" aria-label={`ปลดล็อก ${item.displayName}`}><RotateCcw size={17} /></Button>
            <Button variant="icon" onClick={() => setResetTarget(item)} disabled={item.id === currentUserId} title="ตั้งรหัสผ่านใหม่" aria-label={`ตั้งรหัสผ่านใหม่ให้ ${item.displayName}`}><KeyRound size={17} /></Button>
            <Button variant="icon" danger onClick={() => void setUserActive(item, !item.isActive)} disabled={item.id === currentUserId} title={item.isActive ? "ปิดใช้งาน" : "เปิดใช้งาน"} aria-label={item.isActive ? `ปิดใช้งาน ${item.displayName}` : `เปิดใช้งาน ${item.displayName}`}>{item.isActive ? <UserX size={17} /> : <CheckCircle2 size={17} />}</Button>
            {canHardDelete && <Button variant="icon" danger onClick={() => void hardDeleteUser(item)} disabled={item.id === currentUserId} title="Hard Delete" aria-label={`Hard Delete ${item.displayName}`}><Trash2 size={17} /></Button>}
          </div>
        )} />
      </DataTable>
    )}
    {editor && <UserEditor user={editor === "create" ? undefined : editor} roles={roles} organizations={organizations} defaultOrganizationId={defaultOrganizationId} onClose={() => setEditor(null)} onSaved={() => { setEditor(null); void loadUsers(); }} />}
    {resetTarget && <PasswordResetDialog user={resetTarget} onClose={() => setResetTarget(null)} onSaved={() => { setResetTarget(null); void loadUsers(); }} />}
  </div>;
}

function UserEditor({ user, roles, organizations, defaultOrganizationId, onClose, onSaved }: { user?: ManagedUser; roles: Role[]; organizations: Organization[]; defaultOrganizationId?: string; onClose: () => void; onSaved: () => void }) {
  const [email, setEmail] = useState(user?.email ?? "");
  const [username, setUsername] = useState(user?.username ?? "");
  const [displayName, setDisplayName] = useState(user?.displayName ?? "");
  const [password, setPassword] = useState("");
  const [organizationId, setOrganizationId] = useState(user?.organizationId ?? defaultOrganizationId ?? "");
  const [roleId, setRoleId] = useState(roles.find((role) => user?.roles.includes(role.name))?.id ?? roles[0]?.id ?? "");
  const [isActive, setIsActive] = useState(user?.isActive ?? true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!roleId && roles[0]) setRoleId(roles[0].id);
  }, [roleId, roles]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      if (!organizationId || !roleId) throw new Error("กรุณาระบุ Organization ID และ Role");
      const response = await api(user ? `/api/v1/admin/users/${encodeURIComponent(user.id)}` : "/api/v1/admin/users", {
        method: user ? "PUT" : "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify(user ? { email, username, displayName, roleId, isActive } : { organizationId, email, username, displayName, password, roleId }),
      });
      if (!response.ok) throw new Error(response.status === 409 ? "อีเมลหรือ username นี้มีอยู่แล้ว" : response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์จัดการ User/Role" : "ไม่สามารถบันทึกผู้ใช้ได้");
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
          <div><DialogDescription>User management</DialogDescription><DialogTitle>{user ? "แก้ไขผู้ใช้" : "เพิ่มผู้ใช้"}</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={submit}>
            <label>อีเมล<TextInput type="email" value={email} onChange={(event) => setEmail(event.target.value)} maxLength={320} required /></label>
            <label>Username<TextInput value={username} onChange={(event) => setUsername(event.target.value)} maxLength={100} /></label>
            <label className="full-field">ชื่อแสดงผล<TextInput value={displayName} onChange={(event) => setDisplayName(event.target.value)} maxLength={200} required /></label>
            <label>Organization
              {user || organizations.length === 0 ? (
                <TextInput value={organizationId} onChange={(event) => setOrganizationId(event.target.value)} required disabled={Boolean(user)} />
              ) : (
                <Select value={organizationId} onValueChange={setOrganizationId}>
                  <SelectTrigger><SelectValue placeholder="เลือก Organization" /></SelectTrigger>
                  <SelectContent>{organizations.map((organization) => <SelectItem key={organization.id} value={organization.id}>{organization.name}</SelectItem>)}</SelectContent>
                </Select>
              )}
            </label>
            <label>Role
              <Select value={roleId} onValueChange={setRoleId}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>{roles.map((role) => <SelectItem key={role.id} value={role.id}>{role.name}</SelectItem>)}</SelectContent>
              </Select>
            </label>
            {!user && <label className="full-field">รหัสผ่านเริ่มต้น<PasswordInput autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={12} maxLength={72} required /></label>}
            {user && <label className="toggle-field full-field"><Checkbox checked={isActive} onChange={setIsActive} /> เปิดใช้งาน User</label>}
            {error && <FormMessage className="full-field">{error}</FormMessage>}
            <div className="editor-actions full-field"><Button type="button" variant="secondary" onClick={onClose} disabled={pending}>ยกเลิก</Button><Button disabled={pending}><Save size={17} /> {pending ? "กำลังบันทึก" : "บันทึกผู้ใช้"}</Button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

function PasswordResetDialog({ user, onClose, onSaved }: { user: ManagedUser; onClose: () => void; onSaved: () => void }) {
  const [password, setPassword] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const response = await api(`/api/v1/admin/users/${encodeURIComponent(user.id)}/reset-password`, { method: "POST", headers: { "X-CSRF-Token": csrfToken() }, body: JSON.stringify({ newPassword: password }) });
      if (!response.ok) throw new Error(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์ Reset Password" : "รหัสผ่านไม่ผ่าน policy หรือบันทึกไม่สำเร็จ");
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
          <div><DialogDescription>{user.email}</DialogDescription><DialogTitle>ตั้งรหัสผ่านใหม่</DialogTitle></div>
        </DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={submit}>
            <label className="full-field">รหัสผ่านใหม่<PasswordInput autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={12} maxLength={72} required /></label>
            <p className="full-field text-xs text-slate-500">ทุก session และ reset token ของ User จะถูก revoke</p>
            {error && <FormMessage className="full-field">{error}</FormMessage>}
            <div className="editor-actions full-field"><Button type="button" variant="secondary" onClick={onClose} disabled={pending}>ยกเลิก</Button><Button disabled={pending}><KeyRound size={17} /> {pending ? "กำลังบันทึก" : "ตั้งรหัสผ่าน"}</Button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
