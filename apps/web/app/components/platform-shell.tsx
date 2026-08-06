"use client";

import { LogOut, Menu, SunMedium, UserRound, X } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { createContext, ReactNode, useContext, useEffect, useState } from "react";
import { api, assetURL, csrfToken } from "../lib/api";
import { navigation, navRequirements, titles } from "../lib/navigation";
import { hasPermission } from "../lib/permissions";
import { useRealtimeSocket } from "../lib/realtime";
import { applyAccentColor } from "../lib/theme";
import type { AuthMode, ConnectionState, SiteSettings, User } from "../lib/types";
import { AuthScreen } from "./auth-screen";
import { LivePulse } from "./live-pulse";
import { Toaster } from "./ui/sonner";

const DEFAULT_SITE_SETTINGS: SiteSettings = { siteName: "YGATE", logoUrl: null, accentColor: "teal", updatedAt: "" };

type SessionContext = {
  user: User;
  liveState: ConnectionState;
  lastLiveAt: string | null;
  updateCurrentUser: (user: User) => void;
  siteSettings: SiteSettings;
  updateSiteSettings: (settings: SiteSettings) => void;
};

const PlatformSessionContext = createContext<SessionContext | null>(null);

export function usePlatformSession() {
  const value = useContext(PlatformSessionContext);
  if (!value) throw new Error("usePlatformSession must be used inside PlatformShell");
  return value;
}

export function PlatformShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [authMode, setAuthMode] = useState<AuthMode>("login");
  const [gatewayOnline, setGatewayOnline] = useState(false);
  const [lastLiveAt, setLastLiveAt] = useState<string | null>(null);
  const [navOpen, setNavOpen] = useState(false);
  const [siteSettings, setSiteSettings] = useState<SiteSettings>(DEFAULT_SITE_SETTINGS);

  useEffect(() => {
    const token = new URLSearchParams(window.location.search).get("token");
    const verify = new URLSearchParams(window.location.search).get("verify");
    if (verify) setAuthMode("verify");
    else if (token) setAuthMode("reset");

    void (async () => {
      try {
        const response = await api("/api/v1/auth/me");
        setGatewayOnline(response.status !== 502);
        if (response.ok) setUser((await response.json()) as User);
      } catch {
        setGatewayOnline(false);
      } finally {
        setLoading(false);
      }
    })();

    void (async () => {
      try {
        const response = await api("/api/v1/site-settings");
        if (response.ok) setSiteSettings((await response.json()) as SiteSettings);
      } catch {
        // keep DEFAULT_SITE_SETTINGS
      }
    })();
  }, []);

  useEffect(() => { applyAccentColor(siteSettings.accentColor); }, [siteSettings.accentColor]);

  useEffect(() => {
    if (!user) return;
    const required = navRequirements[pathname];
    if (required && !hasPermission(user, required)) router.replace("/");
  }, [user, pathname, router]);

  const liveState = useRealtimeSocket(undefined, (message) => setLastLiveAt(message.sentAt), Boolean(user));

  async function logout() {
    await api("/api/v1/auth/logout", {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
    });
    setUser(null);
  }

  if (loading) {
    return <main className="loading-screen" aria-label="กำลังโหลด"><LivePulse state="connecting" className="loading-pulse" /><span>YGATE Solar SCADA</span></main>;
  }

  if (!user) {
    return <AuthScreen mode={authMode} gatewayOnline={gatewayOnline} siteSettings={siteSettings} onModeChange={setAuthMode} onLogin={(nextUser) => { setUser(nextUser); setGatewayOnline(true); }} />;
  }

  return (
    <PlatformSessionContext.Provider value={{ user, liveState, lastLiveAt, updateCurrentUser: setUser, siteSettings, updateSiteSettings: setSiteSettings }}>
      <Toaster />
      <main className="app-shell">
        <aside className={navOpen ? "sidebar sidebar-open" : "sidebar"}>
          <div className="brand-lockup">
            <span className="brand-mark">{siteSettings.logoUrl ? <img src={assetURL(siteSettings.logoUrl)} alt="" /> : <SunMedium size={19} />}</span>
            <div><strong>{siteSettings.siteName}</strong><small>Solar SCADA</small></div>
          </div>
          <button className="mobile-close" onClick={() => setNavOpen(false)} title="ปิดเมนู" aria-label="ปิดเมนู"><X size={20} /></button>
          <nav aria-label="เมนูหลัก">
            {navigation.map(({ group, items }) => {
              const visible = items.filter((item) => !item.requires || hasPermission(user, item.requires));
              if (visible.length === 0) return null;
              return (
                <div className="nav-group" key={group}>
                  <p className="nav-group-label">{group}</p>
                  {visible.map(({ href, label, icon: Icon }) => (
                    <Link key={href} href={href} className={pathname === href ? "nav-item active" : "nav-item"} onClick={() => setNavOpen(false)}>
                      <Icon size={18} /> {label}
                    </Link>
                  ))}
                </div>
              );
            })}
          </nav>
          <div className="sidebar-user">
            <span className="avatar"><UserRound size={18} /></span>
            <Link href="/profile" onClick={() => setNavOpen(false)}><strong>{user.displayName}</strong><small>{user.email}</small></Link>
            <button onClick={() => void logout()} title="ออกจากระบบ" aria-label="ออกจากระบบ"><LogOut size={18} /></button>
          </div>
        </aside>
        {navOpen && <button className="nav-scrim" onClick={() => setNavOpen(false)} aria-label="ปิดเมนู" />}

        <section className="workspace">
          <header className="topbar">
            <button className="menu-button" onClick={() => setNavOpen(true)} title="เปิดเมนู" aria-label="เปิดเมนู"><Menu size={20} /></button>
            <div><p>Solar operations</p><h1>{titles[pathname] ?? "YGATE"}</h1></div>
            <div className={`live-chip ${liveState}`}>
              <LivePulse state={liveState} />
              <span>{liveState === "connected" ? "Live" : liveState === "connecting" ? "Connecting" : "Offline"}</span>
            </div>
          </header>
          {children}
        </section>
      </main>
    </PlatformSessionContext.Provider>
  );
}
