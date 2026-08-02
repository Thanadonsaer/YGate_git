"use client";

import { ArrowLeft, CheckCircle2, Eye, EyeOff, KeyRound, ShieldCheck, SunMedium } from "lucide-react";
import { FormEvent, useState } from "react";
import { api, assetURL } from "../lib/api";
import type { AuthMode, SiteSettings, User } from "../lib/types";
import { LivePulse } from "./live-pulse";

export function AuthScreen({
  mode,
  gatewayOnline,
  siteSettings,
  onModeChange,
  onLogin,
}: {
  mode: AuthMode;
  gatewayOnline: boolean;
  siteSettings: SiteSettings;
  onModeChange: (mode: AuthMode) => void;
  onLogin: (user: User) => void;
}) {
  const [identifier, setIdentifier] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    setNotice("");
    try {
      if (mode === "forgot") {
        const response = await api("/api/v1/auth/forgot-password", {
          method: "POST",
          body: JSON.stringify({ email }),
        });
        if (!response.ok) throw new Error("ไม่สามารถส่งคำขอได้");
        setNotice("หากอีเมลนี้อยู่ในระบบ ระบบจะส่งลิงก์ตั้งรหัสผ่านให้");
        return;
      }
      if (mode === "reset") {
        const token = new URLSearchParams(window.location.search).get("token") ?? "";
        const response = await api("/api/v1/auth/reset-password", {
          method: "POST",
          body: JSON.stringify({ token, newPassword: password }),
        });
        if (!response.ok) {
          throw new Error(response.status === 400 ? "ลิงก์หมดอายุหรือรหัสผ่านไม่ผ่านเงื่อนไข" : "ไม่สามารถตั้งรหัสผ่านได้");
        }
        window.history.replaceState({}, "", window.location.pathname);
        setPassword("");
        setNotice("ตั้งรหัสผ่านใหม่แล้ว กรุณาเข้าสู่ระบบ");
        onModeChange("login");
        return;
      }
      const response = await api("/api/v1/auth/login", {
        method: "POST",
        body: JSON.stringify({ identifier, password }),
      });
      if (!response.ok) {
        throw new Error(response.status === 401 ? "อีเมล/ชื่อผู้ใช้ หรือรหัสผ่านไม่ถูกต้อง" : "ไม่สามารถเข้าสู่ระบบได้");
      }
      const result = (await response.json()) as { user: User };
      onLogin(result.user);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
    } finally {
      setPending(false);
    }
  }

  const title = mode === "login" ? "เข้าสู่ระบบ" : mode === "forgot" ? "ลืมรหัสผ่าน" : "ตั้งรหัสผ่านใหม่";
  return (
    <main className="auth-screen">
      <div className="auth-overlay" />
      <LivePulse state="connected" className="auth-pulse" />
      <section className="auth-brand">
        <div className="auth-logo">{siteSettings.logoUrl ? <img src={assetURL(siteSettings.logoUrl)} alt="" /> : <SunMedium size={24} />}<span>{siteSettings.siteName}</span></div>
        <div><p>Solar Operations Platform</p><h1>Solar SCADA</h1><span>ศูนย์กลางติดตามระบบผลิตไฟฟ้าพลังงานแสงอาทิตย์</span></div>
      </section>
      <section className="auth-panel" aria-labelledby="auth-title">
        {mode !== "login" && (
          <button className="back-button" onClick={() => { onModeChange("login"); setError(""); }}>
            <ArrowLeft size={17} /> กลับไปเข้าสู่ระบบ
          </button>
        )}
        <div className="auth-heading"><p>Secure access</p><h2 id="auth-title">{title}</h2></div>
        <form onSubmit={submit}>
          {mode === "login" && <label>อีเมลหรือชื่อผู้ใช้<input autoComplete="username" value={identifier} onChange={(event) => setIdentifier(event.target.value)} required /></label>}
          {mode === "forgot" && <label>อีเมล<input type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></label>}
          {mode !== "forgot" && (
            <label>รหัสผ่าน
              <div className="password-field">
                <input type={showPassword ? "text" : "password"} autoComplete={mode === "login" ? "current-password" : "new-password"} minLength={mode === "reset" ? 12 : undefined} value={password} onChange={(event) => setPassword(event.target.value)} required />
                <button type="button" onClick={() => setShowPassword((value) => !value)} title={showPassword ? "ซ่อนรหัสผ่าน" : "แสดงรหัสผ่าน"} aria-label={showPassword ? "ซ่อนรหัสผ่าน" : "แสดงรหัสผ่าน"}>
                  {showPassword ? <EyeOff size={18} /> : <Eye size={18} />}
                </button>
              </div>
            </label>
          )}
          {error && <p className="form-message error">{error}</p>}
          {notice && <p className="form-message success"><CheckCircle2 size={17} />{notice}</p>}
          <button className="primary-button" disabled={pending}>
            {mode === "login" ? <KeyRound size={18} /> : <ShieldCheck size={18} />}
            {pending ? "กำลังดำเนินการ" : title}
          </button>
        </form>
        {mode === "login" && <button className="text-button" onClick={() => onModeChange("forgot")}>ลืมรหัสผ่าน?</button>}
        <div className="gateway-state"><span className={gatewayOnline ? "dot online" : "dot"} />API Gateway {gatewayOnline ? "online" : "offline"}</div>
      </section>
    </main>
  );
}
