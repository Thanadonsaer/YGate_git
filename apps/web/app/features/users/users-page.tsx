"use client";

import { CheckCircle2, KeyRound, Pencil, Plus, RefreshCw, RotateCcw, Save, Trash2, UserX, X } from "lucide-react";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { api, csrfToken, formatDate } from "../../lib/api";
import type { ManagedUser, Role } from "../../lib/types";

export function UsersPage({ currentUserId, defaultOrganizationId }: { currentUserId: string; defaultOrganizationId?: string }) {
  const [users, setUsers] = useState<ManagedUser[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editor, setEditor] = useState<ManagedUser | "create" | null>(null);
  const [resetTarget, setResetTarget] = useState<ManagedUser | null>(null);

  const loadUsers = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [userResponse, roleResponse] = await Promise.all([api("/api/v1/admin/users"), api("/api/v1/admin/roles")]);
      if (userResponse.status === 403 || roleResponse.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์จัดการผู้ใช้");
      if (!userResponse.ok || !roleResponse.ok) throw new Error("ไม่สามารถโหลดข้อมูลผู้ใช้ได้");
      setUsers((await userResponse.json()) as ManagedUser[]);
      setRoles((await roleResponse.json()) as Role[]);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
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
    if (response.ok) await loadUsers();
    else setError(response.status === 403 ? "บัญชีนี้ไม่มีสิทธิ์เปลี่ยนสถานะผู้ใช้" : "ไม่สามารถเปลี่ยนสถานะผู้ใช้ได้");
  }

  async function unlockUser(target: ManagedUser) {
    const response = await api(`/api/v1/admin/users/${encodeURIComponent(target.id)}/unlock`, {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
    });
    if (response.ok) await loadUsers();
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
        <button className="icon-button" onClick={() => void loadUsers()} title="รีเฟรช" aria-label="รีเฟรชรายการผู้ใช้"><RefreshCw size={18} /></button>
        <button className="primary-button compact" onClick={() => setEditor("create")}><Plus size={18} /> เพิ่มผู้ใช้</button>
      </div>
    </div>
    {error && <p className="form-message error">{error}</p>}
    <div className="user-table" role="table" aria-label="ผู้ใช้">
      <div className="user-row user-head" role="row"><span>ผู้ใช้</span><span>องค์กร</span><span>Role</span><span>Login</span><span>สถานะ</span><span aria-label="คำสั่ง" /></div>
      {!loading && users.map((item) => (
        <div className="user-row" role="row" key={item.id}>
          <div><strong>{item.displayName}</strong><small>{item.email}{item.username ? ` · ${item.username}` : ""}</small></div>
          <div><span>{item.organizationName}</span><small>{item.organizationId}</small></div>
          <div><span>{item.roles.join(", ") || "-"}</span><small>Role และ profile แก้ไขได้</small></div>
          <div><span>{item.failedLoginCount.toLocaleString()} failed</span><small>{item.lockedUntil ? `Locked ${formatDate(item.lockedUntil)}` : `Updated ${formatDate(item.updatedAt)}`}</small></div>
          <span className={item.isActive ? "status active" : "status revoked"}>{item.isActive ? "ใช้งาน" : "ปิดใช้งาน"}</span>
          <div className="row-actions">
            <button className="icon-button" onClick={() => setEditor(item)} disabled={item.id === currentUserId} title="แก้ไข User/Role" aria-label={`แก้ไข ${item.displayName}`}><Pencil size={17} /></button>
            <button className="icon-button" onClick={() => void unlockUser(item)} disabled={!item.lockedUntil && item.failedLoginCount === 0} title="ปลดล็อก" aria-label={`ปลดล็อก ${item.displayName}`}><RotateCcw size={17} /></button>
            <button className="icon-button" onClick={() => setResetTarget(item)} disabled={item.id === currentUserId} title="ตั้งรหัสผ่านใหม่" aria-label={`ตั้งรหัสผ่านใหม่ให้ ${item.displayName}`}><KeyRound size={17} /></button>
            <button className="icon-button danger" onClick={() => void setUserActive(item, !item.isActive)} disabled={item.id === currentUserId} title={item.isActive ? "ปิดใช้งาน" : "เปิดใช้งาน"} aria-label={item.isActive ? `ปิดใช้งาน ${item.displayName}` : `เปิดใช้งาน ${item.displayName}`}>{item.isActive ? <UserX size={17} /> : <CheckCircle2 size={17} />}</button>
            {canHardDelete && <button className="icon-button danger" onClick={() => void hardDeleteUser(item)} disabled={item.id === currentUserId} title="Hard Delete" aria-label={`Hard Delete ${item.displayName}`}><Trash2 size={17} /></button>}
          </div>
        </div>
      ))}
      {loading && <div className="table-state">กำลังโหลดข้อมูลผู้ใช้</div>}
      {!loading && !error && users.length === 0 && <div className="table-state">ยังไม่มีผู้ใช้ในขอบเขตที่คุณเข้าถึงได้</div>}
    </div>
    {editor && <UserEditor user={editor === "create" ? undefined : editor} roles={roles} defaultOrganizationId={defaultOrganizationId} onClose={() => setEditor(null)} onSaved={() => { setEditor(null); void loadUsers(); }} />}
    {resetTarget && <PasswordResetDialog user={resetTarget} onClose={() => setResetTarget(null)} onSaved={() => { setResetTarget(null); void loadUsers(); }} />}
  </div>;
}

function UserEditor({ user, roles, defaultOrganizationId, onClose, onSaved }: { user?: ManagedUser; roles: Role[]; defaultOrganizationId?: string; onClose: () => void; onSaved: () => void }) {
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
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setPending(false);
    }
  }

  return <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (!pending && event.target === event.currentTarget) onClose(); }}>
    <section className="plant-editor user-editor" role="dialog" aria-modal="true" aria-labelledby="user-editor-title">
      <header><div><p>User management</p><h2 id="user-editor-title">{user ? "แก้ไขผู้ใช้" : "เพิ่มผู้ใช้"}</h2></div><button className="icon-button" onClick={onClose} disabled={pending} title="ปิด" aria-label="ปิด"><X size={18} /></button></header>
      <form onSubmit={submit}>
        <label>อีเมล<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} maxLength={320} required /></label>
        <label>Username<input value={username} onChange={(event) => setUsername(event.target.value)} maxLength={100} /></label>
        <label className="full-field">ชื่อแสดงผล<input value={displayName} onChange={(event) => setDisplayName(event.target.value)} maxLength={200} required /></label>
        <label>Organization ID<input value={organizationId} onChange={(event) => setOrganizationId(event.target.value)} required disabled={Boolean(user)} /></label>
        <label>Role<select value={roleId} onChange={(event) => setRoleId(event.target.value)} required>{roles.map((role) => <option key={role.id} value={role.id}>{role.name}</option>)}</select></label>
        {!user && <label className="full-field">รหัสผ่านเริ่มต้น<input type="password" autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={12} maxLength={72} required /></label>}
        {user && <label className="toggle-field full-field"><input type="checkbox" checked={isActive} onChange={(event) => setIsActive(event.target.checked)} /> เปิดใช้งาน User</label>}
        {error && <p className="form-message error full-field">{error}</p>}
        <div className="editor-actions full-field"><button type="button" className="secondary-button" onClick={onClose} disabled={pending}>ยกเลิก</button><button className="primary-button" disabled={pending}><Save size={17} /> {pending ? "กำลังบันทึก" : "บันทึกผู้ใช้"}</button></div>
      </form>
    </section>
  </div>;
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
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setPending(false);
    }
  }
  return <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (!pending && event.target === event.currentTarget) onClose(); }}><section className="plant-editor user-editor" role="dialog" aria-modal="true" aria-labelledby="reset-user-title"><header><div><p>{user.email}</p><h2 id="reset-user-title">ตั้งรหัสผ่านใหม่</h2></div><button className="icon-button" onClick={onClose} disabled={pending} title="ปิด" aria-label="ปิด"><X size={18} /></button></header><form onSubmit={submit}><label className="full-field">รหัสผ่านใหม่<input type="password" autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={12} maxLength={72} required /></label><p className="full-field text-xs text-slate-500">ทุก session และ reset token ของ User จะถูก revoke</p>{error && <p className="form-message error full-field">{error}</p>}<div className="editor-actions full-field"><button type="button" className="secondary-button" onClick={onClose} disabled={pending}>ยกเลิก</button><button className="primary-button" disabled={pending}><KeyRound size={17} /> {pending ? "กำลังบันทึก" : "ตั้งรหัสผ่าน"}</button></div></form></section></div>;
}
