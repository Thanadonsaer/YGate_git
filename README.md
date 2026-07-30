# YGATE Solar SCADA

แพลตฟอร์ม Solar SCADA ส่วนกลางและ Site/Edge Middleware เดิมมีวงจรการพัฒนาและ deploy แยกกัน โดย Central Platform ไม่เปลี่ยน contract ของ `dataItemMap` ที่ Middleware เดิมใช้งาน

## Component ที่รันได้

| Component | Path | Address เริ่มต้น |
| --- | --- | --- |
| Web | `apps/web` | `http://localhost:8080` |
| Go API Gateway | `services/api-gateway` | `http://localhost:44440` |
| Go Platform API | `services/platform-api` | `http://localhost:44441` |
| Site/Edge Middleware | `modbus-api-middleware` | ตั้งค่าแยกต่างหาก |

HTTP และ authenticated WebSocket จาก browser ต้องเข้าผ่าน Gateway ส่วน Platform API ดูแล authentication, session, PostgreSQL migration, audit และ realtime origin validation

## รันบนเครื่องพัฒนา

ดูรายชื่อตัวแปร environment จาก [`deploy/local/.env.example`](deploy/local/.env.example) เก็บ password และ bootstrap credentials จริงไว้ใน shell ปัจจุบันหรือระบบจัดการ secret ขององค์กร ห้าม commit ลง Git

1. รัน Platform API จาก `services/platform-api`: `go run ./cmd/platform-api`
2. รัน Gateway จาก `services/api-gateway`: `go run ./cmd/api-gateway`
3. รัน Web จาก `apps/web`: `npm install` แล้ว `npm run dev`

Health check:

- `GET http://localhost:44440/gateway/healthz`
- `GET http://localhost:44440/readyz`
- `GET http://localhost:44441/readyz`

ดู contract และ architecture decision เพิ่มเติมใน README ของแต่ละ component และ `docs/adr/`

## Deploy Production

- [คู่มือ Manual Build และลากไฟล์ไป Deploy](docs/deploy/manual-production.md)
- [คู่มือ CI/CD ด้วย Jenkins](docs/deploy/jenkins-production.md)

Deploy Central Platform เป็น 3 process ที่ restart แยกกันได้หลัง TLS reverse proxy ขององค์กร ส่วน PostgreSQL และ Site/Edge Middleware มีวงจร deploy แยกต่างหาก

### 1. Build immutable artifacts

Build จาก revision ที่ clean ด้วย Go 1.23 ขึ้นไปและ Node.js 20.9 ขึ้นไป ต้องกำหนด `NEXT_PUBLIC_GATEWAY_URL` ก่อน build Web เพราะ Next.js ฝังค่านี้ลง client bundle

```powershell
Push-Location services/platform-api
go test ./...
New-Item -ItemType Directory -Force dist | Out-Null
go build -trimpath -o dist/platform-api ./cmd/platform-api
Pop-Location

Push-Location services/api-gateway
go test ./...
New-Item -ItemType Directory -Force dist | Out-Null
go build -trimpath -o dist/api-gateway ./cmd/api-gateway
Pop-Location

Push-Location apps/web
npm ci
npm run typecheck
npm run build
Pop-Location
```

Publish artifacts ต่อไปนี้ด้วย release version เดียวกัน:

- `services/platform-api/dist/platform-api`
- `services/api-gateway/dist/api-gateway`
- `apps/web/.next/standalone`
- `packages/api-contracts/platform-api.yaml`

### 2. ตั้งค่า Production environment

เก็บ secrets ใน secret manager ของระบบ deploy ห้ามใส่ใน repository หรือ release artifact การใช้ public URL แบบ same-origin ช่วยลดปัญหา cookie และ CORS

```dotenv
PLATFORM_DATABASE_URL=postgresql://<user>:<password>@<database>:5432/<database>
PLATFORM_HTTP_ADDR=127.0.0.1:44441
PLATFORM_COOKIE_SECURE=true
PLATFORM_WEBSOCKET_ORIGINS=scada.example.com
PLATFORM_OPENAPI_FILE=/opt/ygate/platform-api.yaml

GATEWAY_HTTP_ADDR=127.0.0.1:44440
GATEWAY_PLATFORM_URL=http://127.0.0.1:44441
GATEWAY_ALLOWED_ORIGINS=https://scada.example.com

NEXT_PUBLIC_GATEWAY_URL=https://scada.example.com
HOSTNAME=127.0.0.1
PORT=8080
```

ตั้งค่า reverse proxy:

- `/` ไป Web ที่ `127.0.0.1:8080`
- `/api/*`, `/readyz`, `/healthz` และ `/gateway/*` ไป Gateway ที่ `127.0.0.1:44440`
- เปิด WebSocket upgrade สำหรับ `/api/v1/realtime`
- เปิด public เฉพาะ HTTPS port `443`; เก็บ port `44441` และ `5432` ไว้ใน private network

รันแต่ละ process ผ่าน systemd, container orchestrator หรือ supervisor ที่ส่ง `SIGTERM`, รอ graceful shutdown, restart เมื่อ process ล้ม และเก็บ stdout/stderr logs

### 3. Release อย่างปลอดภัย

1. Backup PostgreSQL และทดสอบว่า restore ได้
2. รัน `platform-admin migrate` เป็น step แยกก่อน deploy binary ใหม่ (apply schema ให้เสร็จและ verify ก่อน cutover แทนที่จะปล่อยให้แต่ละ instance auto-migrate ตอน startup พร้อมกัน) หรือ deploy และ restart Platform API ตรงๆ ซึ่งจะ apply embedded forward-only migrations ตอน startup เช่นกัน
3. รอ `GET /readyz` ผ่าน Gateway สำเร็จก่อนทำขั้นต่อไป
4. Deploy และ restart API Gateway แล้วรอ `GET /gateway/healthz` สำเร็จ
5. Deploy Web standalone artifact และรัน `server.js` โดยกำหนด `HOSTNAME` และ `PORT`
6. Smoke test login, OpenAPI, WebSocket, API แบบไม่แก้ข้อมูล และ test Middleware ingestion หนึ่งครั้ง
7. เฝ้าดู application errors, database health, ingestion rejection counts และ WebSocket disconnects หลัง release

สำหรับ installation ใหม่ ให้รัน `platform-admin bootstrap-user` และ `platform-admin bootstrap-middleware` ครั้งเดียว เก็บ Middleware API key ที่สร้างทันที แล้วลบ bootstrap secrets ออกจาก runtime environment

## CI/CD Pipeline

Repository นี้ยังไม่มี pipeline เฉพาะ vendor หรือ container definition สำหรับ Central Platform สามารถใช้ GitHub Actions, GitLab CI, Jenkins หรือระบบเทียบเท่า โดยมี gate ดังนี้:

คู่มือพร้อมใช้งานสำหรับ Jenkins อยู่ที่ [`docs/deploy/jenkins-production.md`](docs/deploy/jenkins-production.md) และ pipeline definition อยู่ที่ [`Jenkinsfile`](Jenkinsfile)

1. **Validate** ทุก pull request:
   - รัน `go test ./...` ใน Go services ทั้งสองตัว
   - รัน `npm ci`, `npm run typecheck` และ `npm run build` ใน `apps/web`
   - ถ้าต้องการ integration test ให้ใช้ PostgreSQL ชั่วคราวผ่าน `PLATFORM_TEST_DATABASE_URL`
2. **Package** เฉพาะ protected release branch หรือ version tag:
   - สร้าง immutable artifacts ทั้ง 4 รายการข้างต้น
   - กำกับ version ด้วย Git commit SHA หรือ release tag
   - สร้าง checksum และเก็บ previous known-good release
3. **Deploy staging** อัตโนมัติ:
   - inject staging secrets และ public Gateway URL ของ staging
   - deploy ตามลำดับ Platform API → Gateway → Web
   - หยุดทันทีถ้า readiness หรือ smoke test ไม่ผ่าน
4. **Promote production** หลังอนุมัติ:
   - promote artifacts ชุดเดียวกับ staging โดยไม่ build ใหม่
   - backup database
   - deploy ทีละ service พร้อม health gate
5. **Rollback application code** เมื่อ health gate ไม่ผ่าน:
   - คืน Platform API, Gateway และ Web artifacts เวอร์ชันก่อนหน้า
   - ห้ามย้อน `schema_migrations`; migrations เป็น forward-only ดังนั้น release ต้องรองรับ schema ที่ apply ไปแล้ว

Protected-environment secrets ที่ควรมี:

- `PLATFORM_DATABASE_URL`
- SMTP credentials และ `PLATFORM_PASSWORD_RESET_URL` เมื่อเปิด password recovery
- production host/origin values
- deployment credentials, TLS certificate references และ backup credentials

ห้ามใส่ bootstrap passwords, Middleware API keys, `.env`, database dumps หรือ secret ใดๆ ลง CI artifacts และ job logs

## ใช้ Git หรือ CI runner บนเครื่อง Production

Git ทำหน้าที่รับ source code เท่านั้น ไม่ใช่ CI/CD engine หากต้องการให้ production รับงาน deploy อัตโนมัติ ต้องเลือกติดตั้ง runner/agent เพียงหนึ่งระบบ เช่น GitHub Actions self-hosted runner, GitLab Runner หรือ Jenkins agent

### แบบแนะนำ: CI build แล้วส่ง artifact เข้า Production

Production ไม่ต้องติดตั้ง Git หรือ Go compiler และไม่ต้องเปิดสิทธิ์เขียน repository ให้เครื่อง production ติดตั้งเฉพาะ:

- Node.js 20.9 ขึ้นไปสำหรับรัน Next.js standalone server
- systemd หรือ process supervisor ตามระบบปฏิบัติการ
- Nginx, Caddy, IIS หรือ TLS reverse proxy ขององค์กร
- PostgreSQL client tools เช่น `psql` และ `pg_dump` สำหรับ readiness, backup และ restore; PostgreSQL server อาจอยู่เครื่องอื่น
- deployment agent หรือ SSH server สำหรับรับ immutable artifacts จาก CI
- CA certificates และเครื่องมือจัดการ log/monitoring ขององค์กร

Go binaries เป็น native executable จึงไม่ต้องติดตั้ง Go runtime บน production

### แบบ build จาก Git บน Production

ใช้เมื่อยังไม่มี artifact registry เท่านั้น เพราะ build บน production เพิ่ม dependency, network access และความเสี่ยง เครื่อง production ต้องติดตั้งเพิ่ม:

- Git
- Go 1.23 ขึ้นไป
- Node.js 20.9 ขึ้นไปพร้อม npm
- CI runner/agent ที่เลือกใช้
- สิทธิ์อ่าน package registry สำหรับ `go mod download` และ `npm ci`
- เครื่องมือ runtime, reverse proxy, PostgreSQL client และ supervisor ตามรายการแบบแนะนำ

ใช้ dedicated user เช่น `ygate-deploy` ซึ่งไม่มีสิทธิ์ root, ใช้ read-only deploy key/token และจำกัด `sudo` ให้ restart ได้เฉพาะ YGATE services ห้ามให้ production runner ประมวลผล workflow จาก public fork หรือ untrusted pull request

อย่าใช้ `git pull` ทับ directory ที่กำลังรัน ให้ pipeline checkout commit SHA ที่ผ่านการอนุมัติลง release directory ใหม่ เช่น `/opt/ygate/releases/<commit-sha>` จากนั้น build, smoke test และสลับ symlink `/opt/ygate/current` เมื่อทุกอย่างผ่าน วิธีนี้ rollback ได้โดยสลับกลับ previous release

ตัวอย่างลำดับบน production:

```text
CI approval
→ checkout approved commit SHA ลง release directory ใหม่
→ inject secrets จาก secret manager
→ build หรือดาวน์โหลด immutable artifacts
→ backup PostgreSQL
→ restart Platform API และตรวจ /readyz
→ restart Gateway และตรวจ /gateway/healthz
→ restart Web และ smoke test
→ สลับ current release สำเร็จ หรือ rollback ถ้า health gate ไม่ผ่าน
```
