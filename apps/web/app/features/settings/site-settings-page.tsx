"use client";

import { CheckCircle2, ImageIcon, Palette, Save, Trash2, Upload } from "lucide-react";
import { ChangeEvent, FormEvent, useState } from "react";
import { usePlatformSession } from "../../components/platform-shell";
import { api, errorMessage, assetURL, csrfToken } from "../../lib/api";
import { ACCENT_PRESETS } from "../../lib/theme";
import type { AccentColor, SiteSettings } from "../../lib/types";
import { toast } from "../../components/ui/sonner";
import { Button } from "../../components/ui/button";

export function SiteSettingsPage() {
  const { siteSettings, updateSiteSettings } = usePlatformSession();
  const [siteName, setSiteName] = useState(siteSettings.siteName);
  const [accentColor, setAccentColor] = useState<AccentColor>(siteSettings.accentColor);
  const [pending, setPending] = useState(false);
  const [logoPending, setLogoPending] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      const response = await api("/api/v1/site-settings", {
        method: "PUT",
        headers: { "X-CSRF-Token": csrfToken() },
        body: JSON.stringify({ siteName, accentColor }),
      });
      if (!response.ok) throw new Error(response.status === 403 ? "เฉพาะ Platform Admin เท่านั้นที่แก้ไข Site Branding ได้" : "ไม่สามารถบันทึก Site Branding ได้");
      updateSiteSettings((await response.json()) as SiteSettings);
      toast.success("บันทึก Site Branding แล้ว");
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPending(false);
    }
  }

  async function uploadLogo(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    setLogoPending(true);
    setError("");
    try {
      const form = new FormData();
      form.append("logo", file);
      const response = await api("/api/v1/site-settings/logo", {
        method: "POST",
        headers: { "X-CSRF-Token": csrfToken() },
        body: form,
      });
      if (!response.ok) throw new Error(response.status === 400 ? "ไฟล์ต้องเป็นรูปภาพ (png/jpeg/svg/webp/gif) ขนาดไม่เกิน 2 MB" : response.status === 403 ? "เฉพาะ Platform Admin เท่านั้นที่แก้ไข logo ได้" : "อัปโหลด logo ไม่สำเร็จ");
      updateSiteSettings((await response.json()) as SiteSettings);
      toast.success("อัปโหลด logo แล้ว");
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLogoPending(false);
    }
  }

  async function removeLogo() {
    setLogoPending(true);
    setError("");
    try {
      const response = await api("/api/v1/site-settings/logo", {
        method: "DELETE",
        headers: { "X-CSRF-Token": csrfToken() },
      });
      if (!response.ok) throw new Error("ลบ logo ไม่สำเร็จ");
      updateSiteSettings((await response.json()) as SiteSettings);
      toast.success("ลบ logo แล้ว");
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLogoPending(false);
    }
  }

  return <div className="content settings-content">
    <div className="section-heading"><div><p>Site-wide</p><h2>Site Branding</h2></div></div>
    <section className="profile-card">
      <header><Palette size={20} /><div><h3>โลโก้ ชื่อเว็บ และสีหลัก</h3><p>ใช้กับทุกหน้าในระบบ ทั้งหน้า Login และ Sidebar</p></div></header>
      <div className="settings-form-body">
        <div className="logo-preview">
          <span className="brand-mark">{siteSettings.logoUrl ? <img src={assetURL(siteSettings.logoUrl)} alt="" /> : <ImageIcon size={18} />}</span>
          <div className="logo-preview-actions">
            <label className="secondary-button compact" style={{ cursor: "pointer" }} aria-label="อัปโหลด logo">
              {logoPending ? "กำลังอัปโหลด..." : <><Upload size={15} /> อัปโหลด logo</>}
              <input type="file" accept="image/png,image/jpeg,image/svg+xml,image/webp,image/gif" disabled={logoPending} style={{ display: "none" }} onChange={(event) => void uploadLogo(event)} />
            </label>
            {siteSettings.logoUrl && <Button type="button" variant="text" danger compact disabled={logoPending} onClick={() => void removeLogo()}><Trash2 size={15} /> ลบ logo</Button>}
            <small>PNG/JPEG/SVG/WEBP/GIF ไม่เกิน 2 MB</small>
          </div>
        </div>
        <form className="plant-editor-form settings-form" onSubmit={submit}>
          <label className="full-field">ชื่อเว็บไซต์<input value={siteName} onChange={(event) => setSiteName(event.target.value)} maxLength={100} required /></label>
          <div className="full-field">
            <p className="field-label">สีหลักของเว็บ</p>
            <div className="accent-swatches">
              {(Object.keys(ACCENT_PRESETS) as AccentColor[]).map((key) => (
                <button type="button" key={key} className={accentColor === key ? "accent-swatch active" : "accent-swatch"} onClick={() => setAccentColor(key)}>
                  <span className="accent-dot" style={{ background: ACCENT_PRESETS[key].action }} />
                  <span>{ACCENT_PRESETS[key].label}</span>
                  {accentColor === key && <CheckCircle2 size={14} />}
                </button>
              ))}
            </div>
          </div>
          {error && <p className="form-message error full-field">{error}</p>}
          <div className="editor-actions full-field"><Button disabled={pending}><Save size={17} /> {pending ? "กำลังบันทึก" : "บันทึก"}</Button></div>
        </form>
      </div>
    </section>
  </div>;
}
