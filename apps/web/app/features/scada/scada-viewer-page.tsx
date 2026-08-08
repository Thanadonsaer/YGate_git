"use client";

import { FilePlus2, Radio } from "lucide-react";
import { Button } from "../../components/ui/button";
import { FormMessage } from "../../components/ui/form";
import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { api, errorMessage, formatDate } from "../../lib/api";
import { useRealtimeSocket } from "../../lib/realtime";
import { LivePulse } from "../../components/live-pulse";
import { ScadaCanvas } from "./scada-page";
import type { Device, LatestTelemetry, PublishedScadaScreen, ScadaScreenSummary } from "../../lib/types";

export function ScadaViewerPage() {
  const [screens, setScreens] = useState<ScadaScreenSummary[]>([]);
  const [activeId, setActiveId] = useState("");
  const [active, setActive] = useState<PublishedScadaScreen | null>(null);
  const [devices, setDevices] = useState<Device[]>([]);
  const [latestByDevice, setLatestByDevice] = useState<Record<string, LatestTelemetry>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const openScreen = useCallback(async (screen: ScadaScreenSummary) => {
    setActiveId(screen.id);
    setError("");
    try {
      const [publishedResponse, deviceResponse, telemetryResponse] = await Promise.all([
        api(`/api/v1/scada/screens/${encodeURIComponent(screen.id)}/published`),
        api(`/api/v1/plants/${encodeURIComponent(screen.plantId)}/devices`),
        api(`/api/v1/plants/${encodeURIComponent(screen.plantId)}/telemetry/latest`),
      ]);
      if (!publishedResponse.ok) throw new Error("Screen นี้ยังไม่มี Published version");
      setActive((await publishedResponse.json()) as PublishedScadaScreen);
      setDevices(deviceResponse.ok ? (await deviceResponse.json()) as Device[] : []);
      if (telemetryResponse.ok) {
        const readings = (await telemetryResponse.json()) as LatestTelemetry[];
        setLatestByDevice(Object.fromEntries(readings.map((reading) => [reading.deviceId, reading])));
      } else setLatestByDevice({});
    } catch (cause) {
      setActive(null);
      setError(errorMessage(cause));
    }
  }, []);

  const loadScreens = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await api("/api/v1/scada/screens");
      if (response.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดู SCADA Screen");
      if (!response.ok) throw new Error("ไม่สามารถโหลด SCADA Screen ได้");
      const published = ((await response.json()) as ScadaScreenSummary[])
        .filter((screen) => screen.publishedVersion > 0)
        .sort((a, b) => a.plantCode.localeCompare(b.plantCode) || a.name.localeCompare(b.name));
      setScreens(published);
      const keepCurrent = published.find((screen) => screen.id === activeId);
      const next = keepCurrent ?? published[0];
      if (next) await openScreen(next);
      else setActive(null);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
    // activeId intentionally excluded: this only reruns on mount / manual refresh, not on tab switches.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [openScreen]);

  useEffect(() => { void loadScreens(); }, [loadScreens]);

  const liveState = useRealtimeSocket(active?.plantId, (message) => {
    if (message.type === "telemetry.snapshot") {
      setLatestByDevice(Object.fromEntries(message.data.map((reading) => [reading.deviceId, reading])));
    }
  }, Boolean(active));

  const groups: { plantCode: string; plantName: string; items: ScadaScreenSummary[] }[] = [];
  for (const screen of screens) {
    const group = groups.at(-1);
    if (group && group.plantCode === screen.plantCode) group.items.push(screen);
    else groups.push({ plantCode: screen.plantCode, plantName: screen.plantName, items: [screen] });
  }

  return <div className="content scada-content scada-viewer">
    <div className="scada-viewer-head">
      <div><p>Control room</p><h2>{active ? active.name : "SCADA Viewer"}</h2></div>
      {active && <div className={`live-chip ${liveState}`}><LivePulse state={liveState} /><span>{liveState === "connected" ? "Live" : liveState === "connecting" ? "Connecting" : "Offline"}</span></div>}
    </div>
    {error && <FormMessage>{error}</FormMessage>}
    {loading && <div className="table-state">กำลังโหลด SCADA Screens</div>}
    {!loading && screens.length === 0 && !error && (
      <div className="table-state scada-viewer-empty">
        <Radio size={22} />
        <p>ยังไม่มี Screen ที่ Publish</p>
        <Link href="/scada" className="inline-flex items-center justify-center gap-1.5 rounded-[var(--radius-md)] border border-line bg-surface px-2.5 py-1.5 text-xs font-bold text-ink transition hover:bg-canvas focus:outline-none focus-visible:ring-2 focus-visible:ring-focus"><FilePlus2 size={16} /> ไปที่ SCADA Builder</Link>
      </div>
    )}
    {screens.length > 0 && (
      <nav className="scada-tabstrip" aria-label="เลือก SCADA Screen">
        {groups.map((group) => (
          <div className="scada-tabstrip-group" key={group.plantCode}>
            <span className="scada-tabstrip-plant">{group.plantCode}</span>
            {group.items.map((screen) => (
              <Button variant="bare" key={screen.id} className={screen.id === activeId ? "scada-tab active" : "scada-tab"} onClick={() => void openScreen(screen)}>
                {screen.name}
              </Button>
            ))}
          </div>
        ))}
      </nav>
    )}
    {active && (
      <>
        <ScadaCanvas
          key={active.id}
          design={active.design}
          editable={false}
          devices={devices}
          latestByDevice={latestByDevice}
          versions={[]}
          canPublish={false}
          onDesignChange={() => {}}
          onRollback={async () => {}}
          hideInspector
          showMinimap={false}
          locked
        />
        <p className="scada-viewer-meta">Published {formatDate(active.publishedAt)} · v{active.publishedVersion}</p>
      </>
    )}
  </div>;
}
