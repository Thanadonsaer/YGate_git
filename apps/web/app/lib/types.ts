export type User = {
  id: string;
  organizationId?: string;
  email: string;
  displayName: string;
  permissions: string[];
};

export type SelfProfile = User & {
  username: string;
  updatedAt: string;
};

export type AccentColor = "teal" | "indigo" | "emerald" | "amber" | "rose";

export type SiteSettings = {
  siteName: string;
  logoUrl?: string | null;
  accentColor: AccentColor;
  updatedAt: string;
};

export type ManagedUser = {
  id: string;
  organizationId: string;
  organizationName: string;
  email: string;
  username?: string;
  displayName: string;
  status: string;
  isActive: boolean;
  failedLoginCount: number;
  lockedUntil?: string | null;
  roles: string[];
  createdAt: string;
  updatedAt: string;
};

export type Role = {
  id: string;
  organizationId: string | null;
  name: string;
  description: string;
  isSystem: boolean;
};

export type Permission = {
  id: string;
  action: string;
  resourceType: string;
  description: string;
};

export type RoleDetail = Role & {
  permissionIds: string[];
};

export type APIKeyClient = {
  id: string;
  organizationId: string;
  organizationName: string;
  name: string;
  keyPrefix: string;
  autoOnboard: boolean;
  isActive: boolean;
  lastSeenAt?: string | null;
  createdAt: string;
  updatedAt: string;
};

export type CreatedAPIKeyClient = APIKeyClient & { apiKey: string };

export type AuditEvent = {
  id: number;
  organizationId?: string;
  actorUserId?: string;
  actorEmail?: string;
  action: string;
  targetType: string;
  targetId?: string;
  beforeData?: unknown;
  afterData?: unknown;
  sourceIp?: string;
  correlationId?: string;
  occurredAt: string;
};

export type Session = {
  id: string;
  createdAt: string;
  lastSeenAt: string;
  expiresAt: string;
  idleExpiresAt: string;
  revokedAt?: string;
  clientIp?: string;
  userAgent: string;
  current: boolean;
};

export type Plant = {
  id: string;
  organizationId: string;
  organizationName: string;
  code: string;
  name: string;
  timezone: string;
  latitude?: number | null;
  longitude?: number | null;
  installedDcKw?: number | null;
  installedAcKw?: number | null;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
};

export type Device = {
  id: string;
  plantId: string;
  externalId: string;
  name: string;
  deviceModelId: string;
  manufacturer: string;
  model: string;
  deviceType: string;
  sourceTypeId?: number | null;
  modbusHost?: string | null;
  modbusPort?: number | null;
  modbusUnitId: number;
  pollIntervalSeconds: number;
  isActive: boolean;
  updatedAt: string;
};

export type ScadaTelemetryBinding = {
  deviceId: string;
  pointKey: string;
  unit: string;
  decimals: number;
};

export type ScadaDataItem = {
  label: string;
  binding: ScadaTelemetryBinding;
  minAlarm?: number;
  maxAlarm?: number;
};

export type ScadaNodeType = "equipment" | "metric" | "label" | "shape" | "section" | "led" | "clock" | "image" | "table" | "alarms" | "ticker" | "device-summary";

export type ScadaNodeData = {
  label: string;
  equipmentKind?: "solar-panel" | "inverter" | "meter" | "grid";
  binding?: ScadaTelemetryBinding;
  deviceId?: string;
  displayType?: "text" | "gauge" | "progress" | "tank";
  shapeKind?: "rectangle" | "circle" | "triangle" | "diamond" | "hexagon";
  minValue?: number;
  maxValue?: number;
  onValue?: number;
  imageUrl?: string;
  text?: string;
  timezone?: string;
  items?: ScadaDataItem[];
};

export type ScadaDesign = {
  version: 1;
  nodes: Array<{ id: string; type: ScadaNodeType; position: { x: number; y: number }; data: ScadaNodeData; width?: number; height?: number }>;
  edges: Array<{ id: string; source: string; target: string; type: "default" | "smoothstep" }>;
  viewport: { x: number; y: number; zoom: number };
};

export type ScadaScreenSummary = {
  id: string;
  organizationId: string;
  plantId: string;
  plantCode: string;
  plantName: string;
  name: string;
  draftVersion: number;
  publishedVersion: number;
  updatedAt: string;
};

export type ScadaScreen = ScadaScreenSummary & {
  design: ScadaDesign;
  canEdit: boolean;
  canPublish: boolean;
  canManageAccess: boolean;
  canHardDelete: boolean;
};

export type PublishedScadaScreen = ScadaScreenSummary & {
  sourceDraftVersion: number;
  design: ScadaDesign;
  publishedAt: string;
};

export type ScadaScreenVersion = {
  version: number;
  sourceDraftVersion: number;
  publishedBy: string;
  publishedAt: string;
  current: boolean;
};

export type RegisterMetadata = {
  id: string;
  organizationId: string;
  plantId: string;
  deviceId: string;
  addressKey: string;
  displayName: string;
  unit: string;
  dataType: "number" | "boolean" | "text" | "enum";
  scale: number;
  offset: number;
  decimals: number;
  isEnabled: boolean;
  notes: string;
  createdAt: string;
  updatedAt: string;
};

export type DeviceModelOption = {
  id: string;
  organizationId: string;
  manufacturer: string;
  model: string;
  deviceType: string;
  sourceTypeId?: number | null;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
};

export type DeviceModelRegisterMetadata = {
  id: string;
  organizationId: string;
  deviceModelId: string;
  addressKey: string;
  displayName: string;
  unit: string;
  dataType: "number" | "boolean" | "text" | "enum";
  scale: number;
  offset: number;
  decimals: number;
  isEnabled: boolean;
  notes: string;
  modbusFunctionCode?: number | null;
  modbusRegister?: number | null;
  modbusWordOrder?: string | null;
  modbusDataType?: string | null;
  createdAt: string;
  updatedAt: string;
};

export type LatestTelemetry = {
  id: string;
  plantId: string;
  deviceId: string;
  gatewayId: string;
  observedAt: string;
  receivedAt: string;
  dataItemMap: Record<string, number>;
  parameterCount: number;
};

export type AlarmRule = {
  id: string;
  organizationId: string;
  plantId: string;
  deviceId: string;
  pointKey: string;
  label: string;
  minValue?: number | null;
  maxValue?: number | null;
  severity: "warning" | "major" | "critical";
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
};

export type AlarmEvent = {
  id: number;
  organizationId: string;
  plantId: string;
  deviceId: string;
  alarmRuleId: string;
  pointKey: string;
  severity: "warning" | "major" | "critical";
  value: number;
  thresholdMin?: number | null;
  thresholdMax?: number | null;
  breachedAt: string;
  clearedAt?: string | null;
  acknowledgedBy?: string | null;
  acknowledgedAt?: string | null;
  acknowledgedNote?: string | null;
};

export type LiveMessage =
  | { type: "connection.ready" | "connection.heartbeat"; sentAt: string; userId: string }
  | { type: "telemetry.snapshot"; sentAt: string; userId: string; plantId: string; data: LatestTelemetry[] }
  | { type: "alarm.event"; sentAt: string; userId: string; plantId: string; data: AlarmEvent[] };

export type DashboardPlantStatus = {
  plantId: string;
  code: string;
  name: string;
  timezone: string;
  isActive: boolean;
  deviceCount: number;
  activeDeviceCount: number;
  reportingDeviceCount: number;
  staleDeviceCount: number;
  offlineDeviceCount: number;
  lastObservedAt?: string | null;
  communicationStatus: "ONLINE" | "DEGRADED" | "OFFLINE" | "NO_DEVICES" | "DISABLED";
};

export type DashboardOverview = {
  generatedAt: string;
  staleAfterSeconds: number;
  plantCount: number;
  activeDeviceCount: number;
  reportingDeviceCount: number;
  staleDeviceCount: number;
  offlineDeviceCount: number;
  plants: DashboardPlantStatus[];
};

export type DashboardWidget = { id: string; type: string; title: string };
export type DashboardLayoutItem = { i: string; x: number; y: number; w: number; h: number };
export type DashboardLayouts = Record<"lg" | "md" | "sm", DashboardLayoutItem[]>;
export type TimeseriesWidgetConfig = {
  version: 1;
  dataBinding: { plantId: string; deviceId: string; pointKey: string; timeRangeHours: 1 | 6 | 24 | 168 };
  display: { unit: string; decimals: number };
};
export type DashboardWidgetConfigs = { "timeseries-line"?: TimeseriesWidgetConfig };
export type DashboardLayout = {
  id?: string | null;
  version: number;
  registryVersion: number;
  widgets: DashboardWidget[];
  layouts: DashboardLayouts;
  widgetConfigs: DashboardWidgetConfigs;
  canEdit: boolean;
  canPublish: boolean;
  updatedAt?: string | null;
};
export type PublishedDashboardLayout = {
  id?: string | null;
  version: number;
  sourceVersion: number;
  registryVersion: number;
  widgets: DashboardWidget[];
  layouts: DashboardLayouts;
  widgetConfigs: DashboardWidgetConfigs;
  publishedAt?: string | null;
};
export type MiddlewareGateway = {
  id: string;
  organizationId: string;
  organizationName: string;
  name: string;
  siteName: string;
  keyPrefix: string;
  softwareVersion?: string | null;
  autoOnboard: boolean;
  isActive: boolean;
  isOnline: boolean;
  configVersion: number;
  configAppliedVersion: number;
  lastSeenAt?: string | null;
  createdAt: string;
  updatedAt: string;
};

export type CreatedMiddlewareGateway = MiddlewareGateway & { apiKey: string };

export type MiddlewareAddress = {
  addressId: number;
  deviceSetId: number;
  functionCode: number;
  register: number;
  description: string;
  canonicalKey?: string;
  sourceTag?: string;
  factor: number;
  offset?: number;
  dataType: string;
  length?: number;
  wordOrder?: string;
  sourceUnit?: string;
  remark?: string;
  enabled: boolean;
};

export type MiddlewareDeviceSet = {
  deviceSetId: number;
  brandId: number;
  brandName?: string;
  devTypeId?: number;
  devType: string;
  devModel: string;
  addressMode?: string;
  byteOrder?: string;
  wordOrder?: string;
  maxBlockSize?: number;
  addresses: MiddlewareAddress[];
};

export type MiddlewareBrand = {
  brandId: number;
  brandName: string;
  brandDescription?: string;
};

export type MiddlewareConnection = {
  connectionId: number;
  connectionName: string;
  host: string;
  port: number;
  unitId?: number;
  deviceSetId: number;
  devDn?: string;
  deviceName?: string;
  plantCode?: string;
  plantName?: string;
  enabled: boolean;
};

export type MiddlewarePlant = {
  plantCode: string;
  plantName: string;
};

export type MiddlewareConfigSnapshot = {
  version: number;
  brands: MiddlewareBrand[];
  deviceSets: MiddlewareDeviceSet[];
  connections: MiddlewareConnection[];
  plants: MiddlewarePlant[];
};

export type ImportMiddlewareConfigResult = {
  deviceModelsCreated: number;
  deviceModelsReused: number;
  registerMetadataUpserted: number;
  registerMetadataSkipped: number;
  connectionsFound: Array<{ host: string; port: number; unitId: number; deviceModel: string }>;
};

export type MiddlewarePatch = {
  id: string;
  version: string;
  os: string;
  arch: string;
  binaryFilename: string;
  sha256: string;
  fileSizeBytes: number;
  uploadedBy?: string | null;
  createdAt: string;
};

export const MIDDLEWARE_DATA_TYPES = ["U16", "I16", "U32", "I32", "U64", "FLOAT32"] as const;

export type TelemetryHistoryPage = { data: LatestTelemetry[]; nextCursor?: string | null };
export type AuthMode = "login" | "forgot" | "reset";
export type ConnectionState = "connecting" | "connected" | "offline";
