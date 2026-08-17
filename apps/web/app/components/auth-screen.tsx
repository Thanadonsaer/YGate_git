"use client";

import { ArrowLeft, KeyRound, ShieldCheck, SunMedium } from "lucide-react";
import { FormMessage, PasswordInput, TextInput } from "./ui/form";
import { FormEvent, useEffect, useState } from "react";
import { api, errorMessage, assetURL } from "../lib/api";
import type { AuthMode, SiteSettings, User } from "../lib/types";
import { LivePulse } from "./live-pulse";
import { Button } from "./ui/button";
import { loginStatusFromBody, resendVerificationPayload, type LoginStatusCode } from "../lib/auth-status";

export function AuthScreen({
  mode,
  gatewayOnline,
  siteSettings,
  initialError = "",
  onModeChange,
  onLogin,
}: {
  mode: AuthMode;
  gatewayOnline: boolean;
  siteSettings: SiteSettings;
  /** Why the shell sent the visitor here, e.g. an expired session. */
  initialError?: string;
  onModeChange: (mode: AuthMode) => void;
  onLogin: (user: User) => void;
}) {
  const [identifier, setIdentifier] = useState("");
  const [email, setEmail] = useState("");
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState(initialError);
  const [notice, setNotice] = useState("");
  const [loginStatus, setLoginStatus] = useState<LoginStatusCode | null>(null);

  useEffect(() => {
    if (mode !== "verify") return;
    void (async () => {
      const token = new URLSearchParams(window.location.search).get("verify") ?? "";
      const response = await api("/api/v1/auth/verify-email?token=" + encodeURIComponent(token));
      if (response.ok) {
        setNotice("ยืนยันอีเมลแล้ว กรุณารอ Admin อนุมัติสิทธิ์ก่อนเข้าสู่ระบบ");
        window.history.replaceState({}, "", window.location.pathname);
        onModeChange("login");
      } else setError("ลิงก์ยืนยันอีเมลไม่ถูกต้องหรือหมดอายุ");
    })();
  }, [mode, onModeChange]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    setNotice("");
    setLoginStatus(null);
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
      if (mode === "register") {
        const response = await api("/api/v1/auth/register", {
          method: "POST",
          body: JSON.stringify({ email, username, displayName, password }),
        });
        if (!response.ok) throw new Error(response.status === 409 ? "อีเมลหรือ username นี้มีอยู่แล้ว" : response.status === 503 ? "ระบบส่งอีเมลยังไม่พร้อมใช้งาน" : "ข้อมูลสมัครไม่ถูกต้อง");
        setNotice("สมัครสำเร็จ กรุณาตรวจสอบอีเมลเพื่อยืนยันบัญชี จากนั้นรอ Admin ตั้งสิทธิ์");
        return;
      }
      const response = await api("/api/v1/auth/login", {
        method: "POST",
        body: JSON.stringify({ identifier, password }),
      });
      if (!response.ok) {
        if (response.status === 403) {
          const status = loginStatusFromBody(await response.json().catch(() => null));
          if (status) {
            setLoginStatus(status.code);
            setError(status.message);
            return;
          }
        }
        throw new Error(response.status === 401 ? "อีเมล/ชื่อผู้ใช้ หรือรหัสผ่านไม่ถูกต้อง" : "ไม่สามารถเข้าสู่ระบบได้");
      }
      const result = (await response.json()) as { user: User };
      // Login returns the session user, while /auth/me also resolves the
      // organization display name used by scoped admin screens.
      const meResponse = await api("/api/v1/auth/me");
      onLogin(meResponse.ok ? ((await meResponse.json()) as User) : result.user);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPending(false);
    }
  }

  async function resendVerification() {
    setPending(true);
    setError("");
    setNotice("");
    try {
      const response = await api("/api/v1/auth/resend-verification", {
        method: "POST",
        body: JSON.stringify(resendVerificationPayload(identifier)),
      });
      if (!response.ok) throw new Error("ไม่สามารถส่งอีเมลยืนยันได้");
      setNotice("หากบัญชีนี้ยังไม่ได้ยืนยัน ระบบจะส่งอีเมลยืนยันให้");
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPending(false);
    }
  }

  const title = mode === "login" ? "เข้าสู่ระบบ" : mode === "forgot" ? "ลืมรหัสผ่าน" : mode === "reset" ? "ตั้งรหัสผ่านใหม่" : mode === "register" ? "สมัครบัญชี" : "ยืนยันอีเมล";
  return (
    <main className="auth-screen">
      <div className="auth-overlay" />
      <LivePulse state="connected" className="auth-pulse" />
      <section className="auth-brand">
        <div className="auth-logo">{siteSettings.logoUrl ? <img src={assetURL(siteSettings.logoUrl)} alt="" /> : <SunMedium size={24} />}<span>{siteSettings.siteName}</span></div>
        <div><p>Solar Operations Platform</p><h1>Solar SCADA</h1><span>ศูนย์กลางติดตามระบบผลิตไฟฟ้าพลังงานแสงอาทิตย์</span></div>
      </section>
      <section className="auth-panel" aria-labelledby="auth-title">
        {mode !== "login" && mode !== "verify" && (
          <Button variant="bare" className="back-button" onClick={() => { onModeChange("login"); setError(""); }}>
            <ArrowLeft size={17} /> กลับไปเข้าสู่ระบบ
          </Button>
        )}
        <div className="auth-heading"><p>Secure access</p><h2 id="auth-title">{title}</h2></div>
        {mode !== "verify" && <form onSubmit={submit}>
          {mode === "login" && <label>อีเมลหรือชื่อผู้ใช้<TextInput autoComplete="username" value={identifier} onChange={(event) => setIdentifier(event.target.value)} required /></label>}
          {mode === "register" && <><label>อีเมล<TextInput type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></label><label>Username<TextInput autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} /></label><label className="full-field">ชื่อแสดงผล<TextInput value={displayName} onChange={(event) => setDisplayName(event.target.value)} required /></label><p className="full-field text-xs text-slate-500">หลังยืนยันอีเมล กรุณารอ Admin Assign Organization และ Role ก่อนเข้าใช้งาน</p></>}
          {mode === "forgot" && <label>อีเมล<TextInput type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></label>}
          {(mode === "login" || mode === "register" || mode === "reset") && (
            <label>รหัสผ่าน
              <PasswordInput autoComplete={mode === "login" ? "current-password" : "new-password"} minLength={mode === "reset" || mode === "register" ? 12 : undefined} value={password} onChange={(event) => setPassword(event.target.value)} required />
            </label>
          )}
          {error && <FormMessage>{error}</FormMessage>}
          {notice && <FormMessage severity="success">{notice}</FormMessage>}
          <Button disabled={pending}>
            {mode === "login" ? <KeyRound size={18} /> : <ShieldCheck size={18} />}
            {pending ? "กำลังดำเนินการ" : title}
          </Button>
        </form>}
        {mode === "login" && <>
          {loginStatus === "EMAIL_UNVERIFIED" && <Button variant="text" disabled={pending || !identifier.trim()} onClick={() => void resendVerification()}>ส่งอีเมลยืนยันอีกครั้ง</Button>}
          <div className="flex gap-2 justify-between">
            <Button variant="text" onClick={() => onModeChange("forgot")}>ลืมรหัสผ่าน?</Button>
            <Button variant="text" onClick={() => onModeChange("register")}>สมัครบัญชีใหม่</Button>
          </div>

        </>}
        <div className="gateway-state"><span className={gatewayOnline ? "dot online" : "dot"} />API Gateway {gatewayOnline ? "online" : "offline"}</div>
      </section>
    </main>
  );
}
