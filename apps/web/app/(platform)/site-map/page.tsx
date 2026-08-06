"use client";

import dynamic from "next/dynamic";

// react-leaflet manipulates the DOM imperatively (Leaflet owns the map
// container once mounted), which conflicts with React SSR/hydration and
// silently breaks the marker layer if this renders on the server. Load it
// client-only.
const SiteMapPage = dynamic(() => import("../../features/site-map/site-map-page").then((mod) => mod.SiteMapPage), { ssr: false });

export default function Page() {
  return <SiteMapPage />;
}
