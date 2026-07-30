"use client";

import { KeyRound, Save, UserRound } from "lucide-react";
import { FormEvent, useCallback, useEffect, useState } from "react";
import { usePlatformSession } from "../../components/platform-shell";
import { api, csrfToken } from "../../lib/api";
import type { SelfProfile } from "../../lib/types";

export function ProfilePage() {
  const { updateCurrentUser } = usePlatformSession();
  const [profile, setProfile] = useState<SelfProfile | null>(null);
  const [email, setEmail] = useState("");
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [profileMessage, setProfileMessage] = useState("");
  const [passwordMessage, setPasswordMessage] = useState("");
  const [pending, setPending] = useState(false);

  const loadProfile = useCallback(async () => {
    const response = await api("/api/v1/auth/profile");
    if (!response.ok) {
      setProfileMessage("ไม่สามารถโหลดข้อมูลโปรไฟล์ได้");
      return;
    }
    const value = (await response.json()) as SelfProfile;
    setProfile(value);
    setEmail(value.email);
    setUsername(value.username);
    setDisplayName(value.displayName);
  }, []);

  useEffect(() => { void loadProfile(); }, [loadProfile]);

  async function saveProfile(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setProfileMessage("");
    const response = await api("/api/v1/auth/profile", {
      method: "PUT",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ email, username, displayName }),
    });
    if (response.ok) {
      const value = (await response.json()) as SelfProfile;
      setProfile(value);
      updateCurrentUser(value);
      setProfileMessage("บันทึกโปรไฟล์แล้ว");
    } else {
      setProfileMessage(response.status === 409 ? "อีเมลหรือ username นี้มีผู้ใช้งานแล้ว" : "ข้อมูลไม่ถูกต้องหรือไม่สามารถบันทึกได้");
    }
    setPending(false);
  }

  async function changePassword(event: FormEvent) {
    event.preventDefault();
    setPasswordMessage("");
    if (newPassword !== confirmPassword) {
      setPasswordMessage("ยืนยันรหัสผ่านใหม่ไม่ตรงกัน");
      return;
    }
    setPending(true);
    const response = await api("/api/v1/auth/change-password", {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
      body: JSON.stringify({ currentPassword, newPassword }),
    });
    if (response.ok) {
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setPasswordMessage("เปลี่ยนรหัสผ่านแล้ว เซสชันอื่นถูกยกเลิกเรียบร้อย");
    } else {
      setPasswordMessage("รหัสผ่านปัจจุบันไม่ถูกต้อง หรือรหัสผ่านใหม่ไม่ผ่านนโยบาย (อย่างน้อย 12 ตัวอักษร)");
    }
    setPending(false);
  }

  return <div className="content profile-content">
    <div className="section-heading"><div><p>My account</p><h2>โปรไฟล์ของฉัน</h2></div></div>
    <div className="profile-grid">
      <section className="profile-card">
        <header><UserRound size={20} /><div><h3>ข้อมูลส่วนตัว</h3><p>แก้ชื่อ อีเมล และชื่อผู้ใช้สำหรับเข้าสู่ระบบ</p></div></header>
        <form onSubmit={saveProfile}>
          <label>ชื่อที่แสดง<input value={displayName} onChange={(event) => setDisplayName(event.target.value)} maxLength={200} required /></label>
          <label>อีเมล<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} maxLength={320} required /></label>
          <label>Username<input value={username} onChange={(event) => setUsername(event.target.value)} maxLength={100} /></label>
          {profileMessage && <p className="form-message">{profileMessage}</p>}
          <button className="primary-button" disabled={pending || !profile}><Save size={17} /> บันทึกโปรไฟล์</button>
        </form>
      </section>
      <section className="profile-card">
        <header><KeyRound size={20} /><div><h3>เปลี่ยนรหัสผ่าน</h3><p>หลังเปลี่ยนแล้ว เซสชันอื่นจะถูกยกเลิกเพื่อความปลอดภัย</p></div></header>
        <form onSubmit={changePassword}>
          <label>รหัสผ่านปัจจุบัน<input type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} required /></label>
          <label>รหัสผ่านใหม่<input type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} minLength={12} maxLength={72} required /></label>
          <label>ยืนยันรหัสผ่านใหม่<input type="password" autoComplete="new-password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} minLength={12} maxLength={72} required /></label>
          {passwordMessage && <p className="form-message">{passwordMessage}</p>}
          <button className="primary-button" disabled={pending}><KeyRound size={17} /> เปลี่ยนรหัสผ่าน</button>
        </form>
      </section>
    </div>
  </div>;
}
