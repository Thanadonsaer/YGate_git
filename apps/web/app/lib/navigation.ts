import {
  Activity,
  BellRing,
  Building2,
  ChartLine,
  FileText,
  FileBarChart,
  Landmark,
  MapPinned,
  Palette,
  Radio,
  Server,
  Settings2,
  ShieldCheck,
  ShieldEllipsis,
  Users,
  Workflow,
} from "lucide-react";

// Single source of truth for the sidebar: which pages exist, which group they
// sit in, and which permission gates them. Shared with the Roles & Permissions
// editor so admins can see "granting this checks unlocks that page" instead of
// reasoning about raw resource_type/action pairs.
export const navigation = [
  {
    group: "Monitoring",
    items: [
      { href: "/", label: "Portfolio Monitoring", icon: Activity },
      { href: "/plants", label: "Plant & Asset Manager", icon: Building2, requires: "plant:read" },
      { href: "/site-map", label: "GIS Asset Map", icon: MapPinned, requires: "plant:read" },
      { href: "/energy-analysis", label: "Analytics", icon: ChartLine, requires: "plant:read" },
      { href: "/scada/live", label: "SCADA", icon: Radio, requires: "scada_screen:view" },
      { href: "/alarms", label: "Alarms & Event Logbook", icon: BellRing, requires: "alarm:read" },
      { href: "/reports", label: "Reports", icon: FileBarChart, requires: "plant:read" },
    ],
  },
  {
    group: "Assets & Config",
    items: [
      { href: "/middlewares", label: "Middleware Gateways", icon: Server, requires: "middleware_client:read" },
      { href: "/register-metadata", label: "Register Metadata", icon: Settings2, requires: "device_model:read" },
      { href: "/scada", label: "SCADA Builder", icon: Workflow, requires: "scada_screen:edit" },
    ],
  },
  {
    group: "Administration",
    items: [
      { href: "/organizations", label: "Organizations", icon: Landmark, requires: "organization:read" },
      { href: "/users", label: "Users", icon: Users, requires: "user:read" },
      { href: "/roles", label: "Roles & Permissions", icon: ShieldEllipsis, requires: "role:read" },
    ],
  },
  {
    group: "System",
    items: [
      { href: "/openapi", label: "OpenAPI", icon: FileText, requires: "api_contract:read" },
      { href: "/audit", label: "Audit Log", icon: ShieldCheck, requires: "audit:read" },
      { href: "/sessions", label: "Sessions", icon: ShieldCheck, requires: "session:read" },
      { href: "/settings", label: "Site Branding", icon: Palette, requires: "site_setting:update" },
    ],
  }
] satisfies ReadonlyArray<{ group: string; items: ReadonlyArray<{ href: string; label: string; icon: typeof Activity; requires?: string }> }>;

export const navRequirements: Record<string, string> = Object.fromEntries(
  navigation.flatMap(({ items }) => items.filter((item) => item.requires).map((item) => [item.href, item.requires as string])),
);

export const titles: Record<string, string> = {
  "/": "Portfolio Monitoring",
  "/plants": "Plant & Asset Manager",
  "/organizations": "Organization Management",
  "/site-map": "GIS Asset Map",
  "/energy-analysis": "Analytics",
  "/register-metadata": "Register Metadata",
  "/scada": "SCADA Builder",
  "/scada/live": "SCADA",
  "/alarms": "Alarms & Event Logbook",
  "/reports": "Reports",
  "/users": "User Management",
  "/roles": "Roles & Permissions",
  "/middlewares": "Middleware Gateways",
  "/openapi": "OpenAPI Contract",
  "/audit": "Audit Log",
  "/sessions": "My Sessions",
  "/settings": "Site Branding",
  "/profile": "My Profile",
};

// Which permissions (resource_type, and optionally a specific subset of
// actions) apply to each page — the source of truth for the Roles &
// Permissions "pick a menu, then pick what it can do" editor. A page can
// draw from more than one resource_type (Plants manages both plants and
// devices); a resource_type can be split across pages by action (SCADA
// Viewer only needs "view", SCADA Builder needs the editing actions).
// actions omitted = every permission under that resource_type applies.
export type PagePermissionGroup = {
  href: string;
  label: string;
  entries: { resourceType: string; actions?: string[] }[];
};

export const pagePermissionGroups: PagePermissionGroup[] = [
  { href: "/", label: "Portfolio Monitoring", entries: [{ resourceType: "dashboard" }] },
  { href: "/site-map", label: "GIS Asset Map", entries: [{ resourceType: "plant", actions: ["read"] }] },
  { href: "/energy-analysis", label: "Analytics", entries: [{ resourceType: "plant", actions: ["read"] }] },
  { href: "/scada/live", label: "SCADA", entries: [{ resourceType: "scada_screen", actions: ["view"] }] },
  { href: "/alarms", label: "Alarms & Event Logbook", entries: [{ resourceType: "alarm" }] },
  { href: "/reports", label: "Reports", entries: [{ resourceType: "plant", actions: ["read"] }] },
  { href: "/plants", label: "Plant & Asset Manager", entries: [{ resourceType: "plant" }, { resourceType: "device" }] },
  { href: "/organizations", label: "Organizations", entries: [{ resourceType: "organization" }] },
  { href: "/register-metadata", label: "Register Metadata", entries: [{ resourceType: "device_model" }] },
  { href: "/scada", label: "SCADA Builder", entries: [{ resourceType: "scada_screen", actions: ["edit", "publish", "manage_access", "hard_delete"] }] },
  { href: "/users", label: "Users", entries: [{ resourceType: "user" }] },
  { href: "/roles", label: "Roles & Permissions", entries: [{ resourceType: "role" }] },
  {
    href: "/middlewares", label: "Middleware Gateways",
    entries: [{ resourceType: "middleware_client" }, { resourceType: "middleware_config" }, { resourceType: "middleware_plant" }, { resourceType: "middleware_patch" }],
  },
  { href: "/openapi", label: "OpenAPI", entries: [{ resourceType: "api_contract" }] },
  { href: "/audit", label: "Audit Log", entries: [{ resourceType: "audit" }] },
  { href: "/sessions", label: "Sessions", entries: [{ resourceType: "session", actions: ["read"] }] },
  { href: "/settings", label: "Site Branding", entries: [{ resourceType: "site_setting" }] },
];

// resource_type -> the page(s) whose entries reference it, for reverse
// lookups (e.g. "which pages does plant:read affect").
export const pagesByResourceType: Record<string, { label: string; href: string }[]> = (() => {
  const map: Record<string, { label: string; href: string }[]> = {};
  for (const group of pagePermissionGroups) {
    for (const entry of group.entries) {
      const pages = (map[entry.resourceType] ??= []);
      if (!pages.some((page) => page.href === group.href)) pages.push({ label: group.label, href: group.href });
    }
  }
  return map;
})();
