"use client";

import { MapContainer, Marker, Popup, TileLayer } from "react-leaflet";
import { divIcon } from "leaflet";
import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api } from "../../lib/api";
import type { DashboardPlantStatus, Plant } from "../../lib/types";

const statusLabel: Record<DashboardPlantStatus["communicationStatus"], string> = {
  ONLINE: "Online", DEGRADED: "Degraded", OFFLINE: "Offline", NO_DEVICES: "No devices", DISABLED: "Disabled",
};

function markerIcon(status: DashboardPlantStatus["communicationStatus"] | undefined) {
  return divIcon({
    className: "",
    html: `<span class="site-map-marker ${(status ?? "no_devices").toLowerCase()}"></span>`,
    iconSize: [16, 16],
    popupAnchor: [0, -8],
  });
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
      const [plantResponse, overviewResponse] = await Promise.all([api("/api/v1/plants"), api("/api/v1/dashboard/overview")]);
      if (plantResponse.status === 403 || overviewResponse.status === 403) throw new Error("บัญชีนี้ไม่มีสิทธิ์ดูข้อมูลโรงไฟฟ้า");
      if (!plantResponse.ok || !overviewResponse.ok) throw new Error("ไม่สามารถโหลดข้อมูลแผนที่ได้");
      setPlants((await plantResponse.json()) as Plant[]);
      const overview = (await overviewResponse.json()) as { plants: DashboardPlantStatus[] };
      setStatusByPlant(Object.fromEntries(overview.plants.map((item) => [item.plantId, item])));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "เกิดข้อผิดพลาด");
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
      {!loading && !error && missingCount > 0 && <p className="site-map-note">{missingCount} โรงไฟฟ้ายังไม่ได้ระบุพิกัด จึงไม่แสดงบนแผนที่</p>}
      {loading ? (
        <div className="table-state">กำลังโหลดข้อมูล</div>
      ) : (
        <MapContainer className="site-map-canvas" center={center} zoom={located.length > 0 ? 6 : 5} scrollWheelZoom>
          <TileLayer attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors' url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png" />
          {located.map((plant) => {
            const status = statusByPlant[plant.id];
            return (
              <Marker key={plant.id} position={[plant.latitude, plant.longitude]} icon={markerIcon(status?.communicationStatus)}>
                <Popup>
                  <div className="site-map-popup">
                    <p>{plant.code}</p>
                    <h3>{plant.name}</h3>
                    <dl>
                      <dt>สถานะ</dt><dd>{status ? statusLabel[status.communicationStatus] : "ไม่ทราบ"}</dd>
                      <dt>Installed DC</dt><dd>{plant.installedDcKw == null ? "-" : `${plant.installedDcKw.toLocaleString()} kW`}</dd>
                      <dt>Installed AC</dt><dd>{plant.installedAcKw == null ? "-" : `${plant.installedAcKw.toLocaleString()} kW`}</dd>
                    </dl>
                    <Link href="/plants">ดูรายละเอียดโรงไฟฟ้า</Link>
                  </div>
                </Popup>
              </Marker>
            );
          })}
        </MapContainer>
      )}
    </div>
  );
}
