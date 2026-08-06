"use client";

import { MapContainer, Marker, Popup, TileLayer, useMap } from "react-leaflet";
import MarkerClusterGroup from "react-leaflet-cluster";
import { divIcon } from "leaflet";
import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, errorMessage, assetURL } from "../../lib/api";
import type { DashboardPlantStatus, Plant } from "../../lib/types";

const statusLabel: Record<DashboardPlantStatus["communicationStatus"], string> = {
  ONLINE: "Online", DEGRADED: "Degraded", OFFLINE: "Offline", NO_DEVICES: "No devices", DISABLED: "Disabled",
};

function markerIcon(status: DashboardPlantStatus["communicationStatus"] | undefined) {
  const color = status === "ONLINE" ? "#0f6c49" : status === "DEGRADED" ? "#b7791f" : status === "OFFLINE" ? "#b42318" : "#6d767d";
  return divIcon({
    className: "",
    html: "<span class=\"site-map-marker\" style=\"background:" + color + "\"></span>",
    iconSize: [16, 16],
    iconAnchor: [8, 8],
    popupAnchor: [0, -8],
  });
}
function FitBounds({ positions }: { positions: Array<[number, number]> }) {
  const map = useMap();

  useEffect(() => {
    if (positions.length === 1) map.setView(positions[0], 10);
    if (positions.length > 1) map.fitBounds(positions, { padding: [40, 40], maxZoom: 10 });
  }, [map, positions]);

  return null;
}
export function SiteMapPage() {
  const [plants, setPlants] = useState<Plant[]>([]);
  const [statusByPlant, setStatusByPlant] = useState<Record<string, DashboardPlantStatus>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

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

  const located = plants.filter((plant): plant is Plant & { latitude: number; longitude: number } => plant.latitude != null && plant.longitude != null);
  const missingCount = plants.length - located.length;
  const center: [number, number] = located.length > 0
    ? [located.reduce((sum, plant) => sum + plant.latitude, 0) / located.length, located.reduce((sum, plant) => sum + plant.longitude, 0) / located.length]
    : [13.7563, 100.5018];

  return (
    <div className="content site-map-content">
      <div className="section-heading">
        <div><p>Site overview</p><h2>แผนที่โรงไฟฟ้า</h2></div>
      </div>
      {error && <p className="form-message error">{error}</p>}
      {!loading && !error && plants.length === 0 && <p className="site-map-note">บัญชีนี้ไม่มีโรงไฟฟ้าที่เข้าถึงได้เลย (ไม่ใช่ปัญหาพิกัด — ตรวจสิทธิ์ plant:read ของ role)</p>}
      {!loading && !error && missingCount > 0 && <p className="site-map-note">{missingCount}/{plants.length} โรงไฟฟ้ายังไม่ได้ระบุพิกัด จึงไม่แสดงบนแผนที่</p>}
      {loading ? (
        <div className="table-state">กำลังโหลดข้อมูล</div>
      ) : (
        <MapContainer className="site-map-canvas" center={center} zoom={located.length > 0 ? 6 : 5} scrollWheelZoom>
          <TileLayer attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors' url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png" />
          <FitBounds positions={located.map((plant) => [plant.latitude, plant.longitude])} />
          <MarkerClusterGroup chunkedLoading showCoverageOnHover={false}>
          {located.map((plant) => {
            const status = statusByPlant[plant.id];
            return (
              <Marker key={plant.id} position={[plant.latitude, plant.longitude]} icon={markerIcon(status?.communicationStatus)}>
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
