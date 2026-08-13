import type { Metadata } from "next";
import { PrimeReactProvider } from "primereact/api";
import "react-grid-layout/css/styles.css";
import "react-resizable/css/styles.css";
import "./globals.css";
import "@xyflow/react/dist/style.css";
import "leaflet/dist/leaflet.css";
import "react-leaflet-cluster/dist/assets/MarkerCluster.css";
import "react-leaflet-cluster/dist/assets/MarkerCluster.Default.css";

export const metadata: Metadata = {
  title: "YGATE Solar SCADA",
  description: "Solar plant monitoring platform",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="th">
      <body>
        <PrimeReactProvider value={{ unstyled: true }}>
          {children}
        </PrimeReactProvider>
      </body>
    </html>
  );
}
