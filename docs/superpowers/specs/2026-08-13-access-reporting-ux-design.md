# Access, Reporting, and Analytics UX Design

## Goal

Make every management screen reflect the caller's write permissions, make organization scope explicit, and provide calculated multi-device CSV exports.

## Scope

1. Hide create, update, delete, assign, and status-changing controls whenever the signed-in user lacks the matching resource permission. Read access continues to show data only. Server-side authorization remains authoritative.
2. Add email-verification status to the User Management list. My Profile shows the caller's roles read-only; the caller cannot edit their own roles from User Management.
3. Scope Roles & Permissions by organization. System Admin can choose an organization; other users are locked to their own organization.
4. Scope the Plant organization selector the same way: System Admin chooses, other users see their organization in a disabled dropdown.
5. Show an immediate loading overlay across Analytics charts whenever the query-driving filters change, until the replacement dataset is ready.
6. Replace the Reports export flow with one calculated CSV for the selected Plants, Devices, and time range. The CSV contains Plant and Device names, timestamp, every reported parameter after existing scaling/offset processing, and derived kWh values using the Analytics energy calculation.

## Permission Model

The web app will derive abilities from the permissions already returned by `/api/v1/auth/me`. A shared helper receives a resource type and action. Pages use it to conditionally render controls, while all API handlers retain their existing permission checks.

## Organization Scope

System Admin remains the only role that may choose a different organization. The UI determines this from the existing global System Admin role/permission data supplied by the session. For non-System Admin users, creation requests use their organization ID and do not offer another choice.

## Report Data Flow

Reports will fetch only Plant and Device records the caller is already allowed to read, then fetch the existing history endpoint per selected device and time range. Client-side processing reuses Analytics' `toSeries`, scaling metadata, and energy integration rules so the exported values match the chart. A CSV encoder writes a single header row followed by timestamped calculated records; Plant and Device names replace IDs.

## Error Handling and Testing

Permission helper and CSV transformation functions receive focused unit tests. API response additions are covered in auth-service tests. Existing TypeScript typecheck and web tests validate page integration; backend Go tests validate user verification fields and report request authorization.
