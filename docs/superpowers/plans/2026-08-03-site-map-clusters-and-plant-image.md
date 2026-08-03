# Site Map Clusters and Plant Image Implementation Plan

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

Goal: เพิ่ม cluster ใน Site Map และรูปหลัก 1 รูปต่อ Plant พร้อม upload, preview, replace, remove และ API/storage ที่ปลอดภัย

Architecture: ใช้ react-leaflet-cluster ครอบ CircleMarker เดิมเพื่อรวมหมุดโดยไม่เปลี่ยนข้อมูลสถานะ. ฝั่ง platform API เก็บไฟล์ด้วย UUID ใน directory ที่กำหนดผ่าน environment และเก็บ image_url แบบ nullable ใน plant. Frontend อัปโหลดผ่าน endpoint แยกจากการบันทึกข้อมูล Plant เดิม.

Tech Stack: Next.js/React, react-leaflet, react-leaflet-cluster, Go net/http, PostgreSQL/sqlc, filesystem storage, existing API contract YAML.

## Global Constraints

- ไม่เพิ่ม gallery: หนึ่ง Plant มีรูปหลักได้สูงสุดหนึ่งรูป
- รับเฉพาะ PNG, JPEG และ WebP; ขนาดสูงสุด 2 MiB
- ใช้ UUID เป็นชื่อไฟล์และห้ามใช้ชื่อไฟล์จากผู้ใช้เป็น path
- ใช้ plant write permission สำหรับ upload/remove
- ห้ามทำให้ Plant ที่ไม่มีรูปหรือพิกัดใช้งานไม่ได้
- รักษา user changes ที่มีอยู่ใน working tree

---

### Task 1: Add plant image database field and generated query support

Files:
- Create: services/platform-api/internal/database/migrations/000028_plant_image.sql
- Modify: services/platform-api/internal/database/queries/core.sql
- Modify: services/platform-api/internal/database/dbgen/core.sql.go
- Modify: services/platform-api/internal/database/dbgen/models.go
- Test: services/platform-api/internal/core/plants_test.go

Interfaces:
- Produces nullable image URL/storage reference in all Plant read/write rows.

- [ ] Step 1: Write the failing test for Plant image mapping and nullable behavior.
- [ ] Step 2: Run the focused Go test and confirm it fails because the field is absent.
- [ ] Step 3: Add the migration, query column, generated model field, and Plant mapping.
- [ ] Step 4: Run the focused Go test and the platform API database/core tests.
- [ ] Step 5: Commit with message feat: add plant image metadata.

---

### Task 2: Implement secure image storage and Plant image API

Files:
- Create: services/platform-api/internal/core/plant_image.go
- Create: services/platform-api/internal/httpapi/plant_image.go
- Modify: services/platform-api/internal/core/plants.go
- Modify: services/platform-api/internal/httpapi/server.go
- Modify: services/platform-api/internal/config/config.go
- Modify: services/platform-api/cmd/platform-api/main.go
- Test: services/platform-api/internal/core/plant_image_test.go
- Test: services/platform-api/internal/httpapi/plant_image_test.go

Interfaces:
- POST /api/v1/plants/{plantId}/image accepts multipart field image and returns the updated Plant.
- DELETE /api/v1/plants/{plantId}/image clears metadata and removes the stored file.
- GET /api/v1/plants/{plantId}/image/{filename} serves only the stored image for the authorized Plant.
- Core service validates bytes with image type detection, 2 MiB limit, PNG/JPEG/WebP allowlist, UUID filename, and organization/permission checks.

- [ ] Step 1: Write failing tests for unsupported type, oversized data, UUID storage name, ownership denial, and delete behavior.
- [ ] Step 2: Run the focused Go tests and verify the expected failures.
- [ ] Step 3: Implement storage directory configuration, validation, upload, replace, delete, and safe serving.
- [ ] Step 4: Add HTTP multipart parsing with MaxBytesReader and route registration.
- [ ] Step 5: Run focused tests, platform API tests, and gofmt.
- [ ] Step 6: Commit with message feat: add plant image API.

---

### Task 3: Update API contract and frontend Plant types

Files:
- Modify: packages/api-contracts/platform-api.yaml
- Modify: apps/web/app/lib/types.ts
- Modify: apps/web/app/lib/api.ts to reuse the existing assetURL helper
- Test: apps/web typecheck

Interfaces:
- Plant gains optional nullable imageUrl.
- Contract documents upload, delete, and image-serving endpoints, request limits, content types, and responses.
- Frontend can convert the returned image URL to the gateway origin.

- [ ] Step 1: Add the failing type usage in the Plant editor/map popup for imageUrl.
- [ ] Step 2: Run npm run typecheck and confirm the contract/type changes are required.
- [ ] Step 3: Add imageUrl and documented endpoints, reusing the existing asset URL helper.
- [ ] Step 4: Run npm run typecheck and contract consistency checks.
- [ ] Step 5: Commit with message feat: describe plant images in API contract.

---

### Task 4: Add upload, preview, replace, and remove to Plant editor

Files:
- Modify: apps/web/app/features/plants/plants-page.tsx
- Modify: apps/web/app/globals.css
- Test: apps/web typecheck and manual editor check

Interfaces:
- Plant editor shows current image or fallback.
- File selection previews locally before upload.
- Upload calls POST image endpoint with multipart field image.
- Remove calls DELETE image endpoint.
- Upload/remove refreshes the Plant object without changing coordinates or other fields.

- [ ] Step 1: Define the multipart request shape next to the component and use the existing npm run typecheck command as the executable frontend check because this package has no test runner.
- [ ] Step 2: Implement file input validation for type and 2 MiB before sending.
- [ ] Step 3: Implement preview, upload, replace, remove, loading, and error states using existing Button/toast patterns.
- [ ] Step 4: Add minimal CSS for the preview and controls.
- [ ] Step 5: Run npm run typecheck and manually verify create/edit Plant flows.
- [ ] Step 6: Commit with message feat: add plant image controls.

---

### Task 5: Add Site Map marker clustering and Plant image popup

Files:
- Modify: apps/web/package.json
- Modify: apps/web/package-lock.json
- Modify: apps/web/app/features/site-map/site-map-page.tsx
- Modify: apps/web/app/globals.css
- Test: apps/web typecheck and manual browser check

Interfaces:
- Site Map uses cluster grouping for all located Plants.
- Cluster expands on zoom/click.
- Individual markers retain communication status colors.
- Popup displays Plant image when imageUrl exists and a fallback otherwise.
- fitBounds continues to frame all located Plants.

- [ ] Step 1: Add the cluster dependency with the existing package manager and verify the installed version supports the current React/Leaflet versions.
- [ ] Step 2: Write the smallest failing component/type usage for the cluster wrapper and image popup.
- [ ] Step 3: Wrap the CircleMarker list in the cluster component and add cluster styling.
- [ ] Step 4: Add popup image rendering with assetURL and a fallback block.
- [ ] Step 5: Run npm run typecheck, then manually verify clustered nearby Plants, separated markers after zoom, popup image/fallback, and missing-coordinate message.
- [ ] Step 6: Commit with message feat: cluster site map markers.

---

### Task 6: End-to-end verification and documentation

Files:
- Modify: services/platform-api/README.md with the Plant image endpoint and storage configuration
- Modify: apps/web/README.md with the frontend behavior and upload limit
- Test: Go tests, frontend typecheck, production build, manual browser flow

- [ ] Step 1: Run go test ./... in services/platform-api.
- [ ] Step 2: Run npm run typecheck in apps/web.
- [ ] Step 3: Run npm run build in apps/web.
- [ ] Step 4: Verify upload, replace, remove, cluster expansion, popup rendering, permission denial, invalid file rejection, and no-image fallback.
- [ ] Step 5: Inspect git diff and git status to ensure only scoped files changed.
- [ ] Step 6: Commit any scoped documentation or verification fixes with message test: verify plant images and site map clusters.

---

## Self-review

- Spec coverage: clustering, one-image limit, validation, filesystem storage, API endpoints, permissions, popup/fallback, and verification are covered by Tasks 1 through 6.
- Completeness scan: every requirement has a concrete task and command.
- Type consistency: imageUrl is nullable on Plant; upload/delete return updated Plant; frontend consumes the same field; cluster consumes the existing located Plant positions.