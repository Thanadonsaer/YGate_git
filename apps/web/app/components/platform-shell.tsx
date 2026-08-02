"use client";

import {
  Activity,
  BellRing,
  Building2,
  FileText,
  LogOut,
  MapPinned,
  Menu,
  Palette,
  Radio,
  Server,
  Settings2,
  ShieldCheck,
  ShieldEllipsis,
  SunMedium,
  UserRound,
  Users,
  Workflow,
  X,
} from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { createContext, ReactNode, useContext, useEffect, useState } from "react";
import { api, assetURL, csrfToken } from "../lib/api";
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

const navigation = [
  {
    group: "Monitoring",
    items: [
      { href: "/", label: "Overview", icon: Activity },
      { href: "/site-map", label: "Site Map", icon: MapPinned, requires: "plant:read" },
      { href: "/scada/live", label: "SCADA Viewer", icon: Radio, requires: "scada_screen:view" },
      { href: "/alarms", label: "Alarms", icon: BellRing, requires: "alarm:read" },
    ],
  },
  {
    group: "Assets & Config",
    items: [
      { href: "/plants", label: "Plants", icon: Building2, requires: "plant:read" },
      { href: "/register-metadata", label: "Register Metadata", icon: Settings2, requires: "device_model:read" },
      { href: "/scada", label: "SCADA Builder", icon: Workflow, requires: "scada_screen:edit" },
    ],
  },
  {
    group: "Administration",
    items: [
      { href: "/users", label: "Users", icon: Users, requires: "user:read" },
      { href: "/roles", label: "Roles & Permissions", icon: ShieldEllipsis, requires: "role:read" },
      { href: "/middlewares", label: "Middleware Gateways", icon: Server, requires: "middleware_client:read" },
      { href: "/openapi", label: "OpenAPI", icon: FileText, requires: "api_contract:read" },
      { href: "/audit", label: "Audit Log", icon: ShieldCheck, requires: "audit:read" },
      { href: "/sessions", label: "Sessions", icon: ShieldCheck },
      { href: "/settings", label: "Site Branding", icon: Palette, requires: "site_setting:update" },
    ],
  },
] satisfies ReadonlyArray<{ group: string; items: ReadonlyArray<{ href: string; label: string; icon: typeof Activity; requires?: string }> }>;

const navRequirements: Record<string, string> = Object.fromEntries(
  navigation.flatMap(({ items }) => items.filter((item) => item.requires).map((item) => [item.href, item.requires as string])),
);

const titles: Record<string, string> = {
  "/": "System Overview",
  "/plants": "Plant Management",
  "/site-map": "Site Map",
  "/register-metadata": "Register Metadata",
  "/scada": "SCADA Builder",
  "/scada/live": "SCADA Viewer",
  "/alarms": "Alarm Monitoring",
  "/users": "User Management",
  "/roles": "Roles & Permissions",
  "/middlewares": "Middleware Gateways",
  "/openapi": "OpenAPI Contract",
  "/audit": "Audit Log",
  "/sessions": "My Sessions",
  "/settings": "Site Branding",
  "/profile": "My Profile",
};

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
    if (token) setAuthMode("reset");

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
