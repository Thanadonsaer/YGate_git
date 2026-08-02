"use client";

import { KeyRound, Pencil, Save, UserRound } from "lucide-react";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { usePlatformSession } from "../../components/platform-shell";
import { api, csrfToken } from "../../lib/api";
import type { SelfProfile } from "../../lib/types";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogBody } from "../../components/ui/dialog";
import { toast } from "../../components/ui/sonner";

export function ProfilePage() {
  const { updateCurrentUser } = usePlatformSession();
  const [profile, setProfile] = useState<SelfProfile | null>(null);
  const [editingProfile, setEditingProfile] = useState(false);
  const [editingPassword, setEditingPassword] = useState(false);
  const [loadError, setLoadError] = useState("");

  const loadProfile = useCallback(async () => {
    const response = await api("/api/v1/auth/profile");
    if (!response.ok) {
      setLoadError("ไม่สามารถโหลดข้อมูลโปรไฟล์ได้");
      return;
    }
    setProfile((await response.json()) as SelfProfile);
  }, []);

  useEffect(() => { void loadProfile(); }, [loadProfile]);

  return <div className="content profile-content">
    <div className="section-heading"><div><p>My account</p><h2>โปรไฟล์ของฉัน</h2></div></div>
    <div className="profile-grid">
      <section className="profile-card">
        <header>
          <UserRound size={20} /><div><h3>ข้อมูลส่วนตัว</h3><p>ชื่อ อีเมล และชื่อผู้ใช้สำหรับเข้าสู่ระบบ</p></div>
          <button className="icon-button" onClick={() => setEditingProfile(true)} disabled={!profile} title="แก้ไขข้อมูลส่วนตัว" aria-label="แก้ไขข้อมูลส่วนตัว"><Pencil size={17} /></button>
        </header>
        <div className="profile-preview">
          {loadError && <p className="form-message error">{loadError}</p>}
          {profile && <>
            <div><span>ชื่อที่แสดง</span><strong>{profile.displayName}</strong></div>
            <div><span>อีเมล</span><strong>{profile.email}</strong></div>
            <div><span>Username</span><strong>{profile.username || "-"}</strong></div>
          </>}
        </div>
      </section>
      <section className="profile-card">
        <header>
          <KeyRound size={20} /><div><h3>ความปลอดภัย</h3><p>เปลี่ยนรหัสผ่านเข้าสู่ระบบ — หลังเปลี่ยนแล้ว เซสชันอื่นจะถูกยกเลิก</p></div>
        </header>
        <div className="profile-preview">
          <button className="secondary-button" onClick={() => setEditingPassword(true)}><KeyRound size={17} /> เปลี่ยนรหัสผ่าน</button>
        </div>
      </section>
    </div>
    {profile && editingProfile && (
      <EditProfileDialog
        profile={profile}
        onClose={() => setEditingProfile(false)}
        onSaved={(value) => { setProfile(value); updateCurrentUser(value); setEditingProfile(false); toast.success("บันทึกโปรไฟล์แล้ว"); }}
      />
    )}
    {editingPassword && <ChangePasswordDialog onClose={() => setEditingPassword(false)} />}
  </div>;
}

function EditProfileDialog({ profile, onClose, onSaved }: { profile: SelfProfile; onClose: () => void; onSaved: (value: SelfProfile) => void }) {
  const [email, setEmail] = useState(profile.email);
  const [username, setUsername] = useState(profile.username);
  const [displayName, setDisplayName] = useState(profile.displayName);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    const response = await api("/api/v1/auth/profile", {
      method: "PUT",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ email, username, displayName }),
    });
    if (response.ok) onSaved((await response.json()) as SelfProfile);
    else setError(response.status === 409 ? "อีเมลหรือ username นี้มีผู้ใช้งานแล้ว" : "ข้อมูลไม่ถูกต้องหรือไม่สามารถบันทึกได้");
    setPending(false);
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !pending) onClose(); }}>
      <DialogContent className="max-w-lg">
        <DialogHeader><div><DialogDescription>My account</DialogDescription><DialogTitle>แก้ไขข้อมูลส่วนตัว</DialogTitle></div></DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={submit}>
            <label className="full-field">ชื่อที่แสดง<input autoFocus value={displayName} onChange={(event) => setDisplayName(event.target.value)} maxLength={200} required /></label>
            <label className="full-field">อีเมล<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} maxLength={320} required /></label>
            <label className="full-field">Username<input value={username} onChange={(event) => setUsername(event.target.value)} maxLength={100} /></label>
            {error && <p className="form-message error full-field">{error}</p>}
            <div className="editor-actions full-field"><button type="button" className="secondary-button" onClick={onClose} disabled={pending}>ยกเลิก</button><button className="primary-button" disabled={pending}><Save size={17} /> {pending ? "กำลังบันทึก" : "บันทึก"}</button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

function ChangePasswordDialog({ onClose }: { onClose: () => void }) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    if (newPassword !== confirmPassword) {
      setError("ยืนยันรหัสผ่านใหม่ไม่ตรงกัน");
      return;
    }
    setPending(true);
    const response = await api("/api/v1/auth/change-password", {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ currentPassword, newPassword }),
    });
    if (response.ok) {
      toast.success("เปลี่ยนรหัสผ่านแล้ว เซสชันอื่นถูกยกเลิกเรียบร้อย");
      onClose();
    } else {
      setError("รหัสผ่านปัจจุบันไม่ถูกต้อง หรือรหัสผ่านใหม่ไม่ผ่านนโยบาย (อย่างน้อย 12 ตัวอักษร)");
    }
    setPending(false);
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !pending) onClose(); }}>
      <DialogContent className="max-w-lg">
        <DialogHeader><div><DialogDescription>My account</DialogDescription><DialogTitle>เปลี่ยนรหัสผ่าน</DialogTitle></div></DialogHeader>
        <DialogBody>
          <form className="plant-editor-form" onSubmit={submit}>
            <label className="full-field">รหัสผ่านปัจจุบัน<input autoFocus type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} required /></label>
            <label className="full-field">รหัสผ่านใหม่<input type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} minLength={12} maxLength={72} required /></label>
            <label className="full-field">ยืนยันรหัสผ่านใหม่<input type="password" autoComplete="new-password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} minLength={12} maxLength={72} required /></label>
            {error && <p className="form-message error full-field">{error}</p>}
            <div className="editor-actions full-field"><button type="button" className="secondary-button" onClick={onClose} disabled={pending}>ยกเลิก</button><button className="primary-button" disabled={pending}><KeyRound size={17} /> {pending ? "กำลังเปลี่ยน" : "เปลี่ยนรหัสผ่าน"}</button></div>
          </form>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
