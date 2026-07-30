"use client";

import {
  Activity,
  BellRing,
  Building2,
  FileText,
  KeyRound,
  LogOut,
  MapPinned,
  Menu,
  Settings2,
  ShieldCheck,
  ShieldEllipsis,
  SunMedium,
  UserRound,
  UserCog,
  Users,
  Wifi,
  WifiOff,
  Workflow,
  X,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { createContext, ReactNode, useContext, useEffect, useState } from "react";
import { api, csrfToken } from "../lib/api";
import { useRealtimeSocket } from "../lib/realtime";
import type { AuthMode, ConnectionState, User } from "../lib/types";
import { AuthScreen } from "./auth-screen";

type SessionContext = {
  user: User;
  liveState: ConnectionState;
  lastLiveAt: string | null;
  updateCurrentUser: (user: User) => void;
};

const PlatformSessionContext = createContext<SessionContext | null>(null);

export function usePlatformSession() {
  const value = useContext(PlatformSessionContext);
  if (!value) throw new Error("usePlatformSession must be used inside PlatformShell");
  return value;
}

const navigation = [
  { href: "/", label: "Overview", icon: Activity },
  { href: "/plants", label: "Plants", icon: Building2 },
  { href: "/site-map", label: "Site Map", icon: MapPinned },
  { href: "/register-metadata", label: "Register Metadata", icon: Settings2 },
  { href: "/scada", label: "SCADA Screens", icon: Workflow },
  { href: "/alarms", label: "Alarms", icon: BellRing },
  { href: "/users", label: "Users", icon: Users },
  { href: "/roles", label: "Roles & Permissions", icon: ShieldEllipsis },
  { href: "/api-keys", label: "API Keys", icon: KeyRound },
  { href: "/openapi", label: "OpenAPI", icon: FileText },
  { href: "/audit", label: "Audit Log", icon: ShieldCheck },
  { href: "/sessions", label: "Sessions", icon: ShieldCheck },
  { href: "/profile", label: "My Profile", icon: UserCog },
] as const;

const titles: Record<string, string> = {
  "/": "System Overview",
  "/plants": "Plant Management",
  "/site-map": "Site Map",
  "/register-metadata": "Register Metadata",
  "/scada": "SCADA Builder & Viewer",
  "/alarms": "Alarm Monitoring",
  "/users": "User Management",
  "/roles": "Roles & Permissions",
  "/api-keys": "API Keys",
  "/openapi": "OpenAPI Contract",
  "/audit": "Audit Log",
  "/sessions": "My Sessions",
  "/profile": "My Profile",
};

export function PlatformShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [authMode, setAuthMode] = useState<AuthMode>("login");
  const [gatewayOnline, setGatewayOnline] = useState(false);
  const [lastLiveAt, setLastLiveAt] = useState<string | null>(null);
  const [navOpen, setNavOpen] = useState(false);

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
  }, []);

  const liveState = useRealtimeSocket(undefined, (message) => setLastLiveAt(message.sentAt), Boolean(user));

  async function logout() {
    await api("/api/v1/auth/logout", {
      method: "POST",
      headers: { "X-CSRF-Token": csrfToken() },
    });
    setUser(null);
  }

  if (loading) {
    return <main className="loading-screen" aria-label="กำลังโหลด"><SunMedium size={30} /><span>YGATE Solar SCADA</span></main>;
  }

  if (!user) {
    return <AuthScreen mode={authMode} gatewayOnline={gatewayOnline} onModeChange={setAuthMode} onLogin={(nextUser) => { setUser(nextUser); setGatewayOnline(true); }} />;
  }

  return (
    <PlatformSessionContext.Provider value={{ user, liveState, lastLiveAt, updateCurrentUser: setUser }}>
      <main className="app-shell">
        <aside className={navOpen ? "sidebar sidebar-open" : "sidebar"}>
          <div className="brand-lockup">
            <span className="brand-mark"><SunMedium size={19} /></span>
            <div><strong>YGATE</strong><small>Solar SCADA</small></div>
          </div>
          <button className="mobile-close" onClick={() => setNavOpen(false)} title="ปิดเมนู" aria-label="ปิดเมนู"><X size={20} /></button>
          <nav aria-label="เมนูหลัก">
            {navigation.map(({ href, label, icon: Icon }) => (
              <Link key={href} href={href} className={pathname === href ? "nav-item active" : "nav-item"} onClick={() => setNavOpen(false)}>
                <Icon size={18} /> {label}
              </Link>
            ))}
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
              {liveState === "connected" ? <Wifi size={15} /> : <WifiOff size={15} />}
              <span>{liveState === "connected" ? "Live" : liveState === "connecting" ? "Connecting" : "Offline"}</span>
            </div>
          </header>
          {children}
        </section>
      </main>
    </PlatformSessionContext.Provider>
  );
}
