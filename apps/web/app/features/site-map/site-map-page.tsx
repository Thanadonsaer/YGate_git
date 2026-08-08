"use client";

import { MapContainer, Marker, Popup, TileLayer, useMap } from "react-leaflet";
import MarkerClusterGroup from "react-leaflet-cluster";
import { divIcon, point } from "leaflet";
import { SelectButton } from "primereact/selectbutton";
import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { FormMessage } from "../../components/ui/form";
import { cn } from "../../lib/cn";
import { api, errorMessage, assetURL } from "../../lib/api";
import type { DashboardPlantStatus, Plant } from "../../lib/types";

type CommunicationStatus = DashboardPlantStatus["communicationStatus"];

const statusLabel: Record<CommunicationStatus, string> = {
  ONLINE: "Online", DEGRADED: "Degraded", OFFLINE: "Offline", NO_DEVICES: "No devices", DISABLED: "Disabled",
};

/** Pin colour per status. The legend reads the same map, so the two can't drift. */
const statusColor: Record<CommunicationStatus | "UNKNOWN", string> = {
  ONLINE: "#0f6c49", DEGRADED: "#b7791f", OFFLINE: "#b42318", NO_DEVICES: "#6d767d", DISABLED: "#6d767d", UNKNOWN: "#6d767d",
};

// {s} subdomains are only valid where the provider actually serves them --
// Esri does not, so its template is single-host on purpose.
const BASE_LAYERS = [
  {
    id: "street",
    label: "แผนที่",
    url: "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png",
    attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
    maxZoom: 19,
  },
  {
    id: "satellite",
    label: "ดาวเทียม",
    url: "https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}",
    attribution: "Tiles &copy; Esri — Source: Esri, Maxar, Earthstar Geographics",
    maxZoom: 19,
  },
  {
    id: "terrain",
    label: "ภูมิประเทศ",
    url: "https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png",
    attribution: 'Map data: &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>, SRTM | &copy; <a href="https://opentopomap.org">OpenTopoMap</a>',
    maxZoom: 17,
  },
  {
    id: "dark",
    label: "โหมดมืด",
    url: "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png",
    attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>',
    maxZoom: 20,
  },
] as const;

type BaseLayerId = (typeof BASE_LAYERS)[number]["id"];

function markerIcon(status: CommunicationStatus | undefined) {
  const tone = (status ?? "UNKNOWN").toLowerCase();
  return divIcon({
    className: "site-pin-icon",
    html:
      `<span class="site-pin site-pin-${tone}" style="--pin:${statusColor[status ?? "UNKNOWN"]}">`
      + '<svg viewBox="0 0 30 42" aria-hidden="true">'
      + '<path d="M15 41S28 25.9 28 15.2C28 7.4 22.2 1 15 1S2 7.4 2 15.2C2 25.9 15 41 15 41Z"/>'
      + '<circle cx="15" cy="15.2" r="5.4"/>'
      + "</svg></span>",
    iconSize: [30, 42],
    iconAnchor: [15, 42],
    popupAnchor: [0, -38],
  });
}

// Structural type: leaflet.markercluster augments the L namespace rather than
// exporting MarkerCluster, and getChildCount is all this needs.
function clusterIcon(cluster: { getChildCount: () => number }) {
  const count = cluster.getChildCount();
  const size = count < 10 ? 40 : count < 100 ? 48 : 56;
  return divIcon({
    className: "site-cluster-icon",
    html: `<span class="site-cluster"><b>${count}</b></span>`,
    iconSize: point(size, size, true),
  });
}

function FitBounds({ positions }: { positions: Array<[number, number]> }) {
  const map = useMap();

  useEffect(() => {
    if (positions.length === 1) map.setView(positions[0], 10);
    if (positions.length > 1) map.fitBounds(positions, { padding: [60, 60], maxZoom: 10 });
  }, [map, positions]);

  return null;
}

export function SiteMapPage() {
  const [plants, setPlants] = useState<Plant[]>([]);
  const [statusByPlant, setStatusByPlant] = useState<Record<string, DashboardPlantStatus>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [layerId, setLayerId] = useState<BaseLayerId>("street");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      // Plant locations only need plant:read. The dashboard overview call (used
      // just to color pins by communication status) needs device:read on top of
      // that — a role can legitimately have one without the other, so its
      // failure must not block the map itself, only the status coloring.
      const [plantResponse, overviewResponse] = await Promise.all([api("/api/v1/plants"), api("/api/v1/dashboard/overview")]);
      if (plantResponse.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดูข้อมูลโรงไฟฟ้า");
      if (!plantResponse.ok) throw new Error("ไม่สามารถโหลดข้อมูลแผนที่ได้");
      setPlants((await plantResponse.json()) as Plant[]);
      if (overviewResponse.ok) {
        const overview = (await overviewResponse.json()) as { plants: DashboardPlantStatus[] };
        setStatusByPlant(Object.fromEntries(overview.plants.map((item) => [item.plantId, item])));
      } else {
        setStatusByPlant({});
      }
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void load(); }, [load]);

  const located = useMemo(
    () => plants.filter((plant): plant is Plant & { latitude: number; longitude: number } => plant.latitude != null && plant.longitude != null),
    [plants],
  );
  const missingCount = plants.length - located.length;
  const center: [number, number] = located.length > 0
    ? [located.reduce((sum, plant) => sum + plant.latitude, 0) / located.length, located.reduce((sum, plant) => sum + plant.longitude, 0) / located.length]
    : [13.7563, 100.5018];

  const layer = BASE_LAYERS.find((entry) => entry.id === layerId) ?? BASE_LAYERS[0];

  // Legend counts come from the same map the pins read, so a status with no
  // plants disappears instead of showing a permanent zero.
  const counts = useMemo(() => {
    const tally = new Map<CommunicationStatus | "UNKNOWN", number>();
    for (const plant of located) {
      const key = statusByPlant[plant.id]?.communicationStatus ?? "UNKNOWN";
      tally.set(key, (tally.get(key) ?? 0) + 1);
    }
    return tally;
  }, [located, statusByPlant]);

  return (
    <div className="content site-map-content">
      <div className="section-heading">
        <div><p>Site overview</p><h2>แผนที่โรงไฟฟ้า</h2></div>
        <SelectButton
          value={layerId}
          options={BASE_LAYERS.map((entry) => ({ label: entry.label, value: entry.id }))}
          optionLabel="label"
          optionValue="value"
          allowEmpty={false}
          onChange={(event) => setLayerId(event.value as BaseLayerId)}
          aria-label="เลือกรูปแบบแผนที่"
          pt={{
            root: { className: "inline-flex h-10 items-center gap-0.5 rounded-[var(--radius-sm)] border border-line bg-canvas p-1" },
            button: (options: { context: { selected: boolean } }) => ({
              className: cn(
                "grid h-[30px] cursor-pointer place-items-center rounded-[calc(var(--radius-sm)-2px)] px-3 text-xs font-bold transition",
                options.context.selected ? "bg-surface text-nav shadow-[var(--shadow-sm)]" : "text-ink-soft hover:text-ink",
              ),
            }),
          }}
        />
      </div>
      {error && <FormMessage>{error}</FormMessage>}
      {!loading && !error && plants.length === 0 && <p className="site-map-note">บัญชีนี้ไม่มีโรงไฟฟ้าที่เข้าถึงได้เลย (ไม่ใช่ปัญหาพิกัด — ตรวจสิทธิ์ plant:read ของ role)</p>}
      {!loading && !error && missingCount > 0 && <p className="site-map-note">{missingCount}/{plants.length} โรงไฟฟ้ายังไม่ได้ระบุพิกัด จึงไม่แสดงบนแผนที่</p>}
      {counts.size > 0 && (
        <ul className="site-map-legend">
          {[...counts].map(([status, count]) => (
            <li key={status}>
              <span className="site-map-legend-dot" style={{ background: statusColor[status] }} />
              {status === "UNKNOWN" ? "ไม่ทราบสถานะ" : statusLabel[status]}
              <b>{count}</b>
            </li>
          ))}
        </ul>
      )}
      {loading ? (
        <div className="table-state">กำลังโหลดข้อมูล</div>
      ) : (
        <MapContainer className="site-map-canvas" center={center} zoom={located.length > 0 ? 6 : 5} scrollWheelZoom>
          {/* Keyed so switching base layers mounts a fresh tile source instead
              of mutating the live one, which leaves stale tiles behind. */}
          <TileLayer key={layer.id} attribution={layer.attribution} url={layer.url} maxZoom={layer.maxZoom} />
          <FitBounds positions={located.map((plant) => [plant.latitude, plant.longitude])} />
          <MarkerClusterGroup chunkedLoading showCoverageOnHover={false} iconCreateFunction={clusterIcon} maxClusterRadius={48}>
          {located.map((plant) => {
            const status = statusByPlant[plant.id];
            return (
              <Marker key={plant.id} position={[plant.latitude, plant.longitude]} icon={markerIcon(status?.communicationStatus)} title={plant.name}>
                <Popup>
                  <div className="site-map-popup">
                    <p>{plant.code}</p>
                    <h3>{plant.name}</h3>
                    {plant.imageUrl ? <img className="site-map-popup-image" src={assetURL(plant.imageUrl)} alt={"รูป " + plant.name} /> : <div className="site-map-popup-fallback">ไม่มีรูปโรงไฟฟ้า</div>}
                    <dl>
                      <dt>สถานะ</dt><dd>{status ? statusLabel[status.communicationStatus] : "ไม่ทราบ"}</dd>
                      <dt>Installed DC</dt><dd>{plant.installedDcKw == null ? "-" : `${plant.installedDcKw.toLocaleString()} kW`}</dd>
                      <dt>Installed AC</dt><dd>{plant.installedAcKw == null ? "-" : `${plant.installedAcKw.toLocaleString()} kW`}</dd>
                      {status && (
                        <>
                          <dt>Device</dt><dd>{status.reportingDeviceCount}/{status.deviceCount} รายงานอยู่{status.staleDeviceCount > 0 ? ` (${status.staleDeviceCount} ค้าง)` : ""}{status.offlineDeviceCount > 0 ? ` (${status.offlineDeviceCount} offline)` : ""}</dd>
                          <dt>ข้อมูลล่าสุด</dt><dd>{status.lastObservedAt ? new Date(status.lastObservedAt).toLocaleString("th-TH") : "ไม่มีข้อมูล"}</dd>
                        </>
                      )}
                    </dl>
                    <Link href={`/plants?open=${encodeURIComponent(plant.id)}`}>ดู Overview โรงไฟฟ้านี้</Link>
                  </div>
                </Popup>
              </Marker>
            );
          })}
          </MarkerClusterGroup>
        </MapContainer>
      )}
    </div>
  );
}
