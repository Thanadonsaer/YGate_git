"use client";

import { LogOut, Menu, SunMedium, UserRound, X } from "lucide-react";
import { Button } from "./ui/button";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { createContext, ReactNode, useContext, useEffect, useRef, useState } from "react";
import { api, assetURL, csrfToken, onSessionExpired } from "../lib/api";
import { navigation, navRequirements, titles } from "../lib/navigation";
import { hasPermission } from "../lib/permissions";
import { useRealtimeSocket } from "../lib/realtime";
import { applyAccentColor } from "../lib/theme";
import type { AuthMode, ConnectionState, SiteSettings, User } from "../lib/types";
import { AuthScreen } from "./auth-screen";
import { LivePulse } from "./live-pulse";
import { Toaster } from "./ui/sonner";

const DEFAULT_SITE_SETTINGS: SiteSettings = { siteName: "YGATE", logoUrl: null, accentColor: "teal", updatedAt: "" };
const BUILD_VERSION = process.env.NEXT_PUBLIC_APP_VERSION || "dev";

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
  const [sessionExpired, setSessionExpired] = useState(false);
  const signedIn = useRef(false);
  signedIn.current = Boolean(user);

  // The session cookie can lapse long after the initial /auth/me, so drop back
  // to the login screen the moment any request comes back 401 instead of
  // leaving a shell that looks signed in but can no longer load anything.
  // ponytail: reacts to the next request, so an idle tab keeps showing the
  // shell until something is clicked; add an /auth/me heartbeat if that matters.
  useEffect(() =>
    onSessionExpired(() => {
      // A 401 before anyone signed in is just the logged-out first visit.
      if (!signedIn.current) return;
      setUser(null);
      setSessionExpired(true);
    }), []);

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
    return <AuthScreen mode={authMode} gatewayOnline={gatewayOnline} siteSettings={siteSettings} initialError={sessionExpired ? "เซสชันหมดอายุ กรุณาเข้าสู่ระบบอีกครั้ง" : ""} onModeChange={setAuthMode} onLogin={(nextUser) => { setUser(nextUser); setGatewayOnline(true); setSessionExpired(false); }} />;
  }

  return (
    <PlatformSessionContext.Provider value={{ user, liveState, lastLiveAt, updateCurrentUser: setUser, siteSettings, updateSiteSettings: setSiteSettings }}>
      <Toaster />
      <main className="app-shell">
        <aside className={navOpen ? "sidebar sidebar-open" : "sidebar"}>
          <div className="brand-lockup">
            <span className="brand-mark">{siteSettings.logoUrl ? <img src={assetURL(siteSettings.logoUrl)} alt="" /> : <SunMedium size={19} />}</span>
            <div><strong>{siteSettings.siteName}</strong><small>Solar SCADA · v{BUILD_VERSION}</small></div>
          </div>
          <Button variant="bare" className="mobile-close" onClick={() => setNavOpen(false)} title="ปิดเมนู" aria-label="ปิดเมนู"><X size={20} /></Button>
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
            <Button variant="bare" onClick={() => void logout()} title="ออกจากระบบ" aria-label="ออกจากระบบ"><LogOut size={18} /></Button>
          </div>
        </aside>
        {navOpen && <Button variant="bare" className="nav-scrim" onClick={() => setNavOpen(false)} aria-label="ปิดเมนู" />}

        <section className="workspace">
          <header className="topbar">
            <Button variant="bare" className="menu-button" onClick={() => setNavOpen(true)} title="เปิดเมนู" aria-label="เปิดเมนู"><Menu size={20} /></Button>
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
