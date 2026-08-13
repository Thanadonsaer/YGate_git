# คู่มือ CI/CD: Git → Jenkins → Production (Windows + PM2)

Jenkins ติดตั้งอยู่บนเครื่อง production เดียวกัน (Windows amd64) จึงไม่มีการ SSH ข้ามเครื่อง Pipeline เรียกสคริปต์ build/start ตัวเดียวกับที่ใช้ manual deploy ([manual-production.md](manual-production.md)) เพื่อไม่ให้ logic build/start แยกกันสองชุด

```text
Git push / merge main
        ↓ webhook (ผ่าน Cloudflare Tunnel)
Jenkins checkout → test → build-release.ps1 (build + package)
        ↓ manual approval
Expand-Archive ลง release directory ใหม่ → start.ps1 (migrate + pm2 + health check)
```

พื้นฐาน service layout, PostgreSQL, Cloudflare Tunnel ดู [manual-production.md](manual-production.md) ก่อน เอกสารนี้พูดเฉพาะส่วนที่เป็น Jenkins

## 1. เตรียมเครื่อง Jenkins (= เครื่อง production)

ติดตั้งบนเครื่องเดียวกับที่รัน PM2:

- Jenkins LTS (Windows service) และ Java 21
- Git, Go 1.23+, Node.js 20.9+ พร้อม npm, `tar.exe` — ตัวเดียวกับที่ [build-release.ps1](../../deploy/manual/build-release.ps1) ต้องการ
- PM2 (`npm install -g pm2`) — ใช้รันจริงตอน deploy
- Jenkins service account ต้องมองเห็นทุกอย่างข้างบนใน PATH (ปกติ Windows service รันเป็น user แยกจาก user ที่ login ใช้งาน ต้องเช็ค PATH ของ service account เอง ไม่ใช่ PATH ตอน login)

เปิด Jenkins ผ่าน Cloudflare Tunnel เท่านั้น ห้ามเปิด port Jenkins (`8000` ในตัวอย่างนี้) ออก internet ตรงๆ

## 2. ติดตั้ง Jenkins plugins

**Manage Jenkins → Plugins**:

- Pipeline
- Git
- GitHub (สำหรับรับ webhook)

ไม่ต้องมี SSH Agent / Credentials Binding เพราะไม่มีการ SSH ข้ามเครื่องอีกต่อไป

## 3. เชื่อม Jenkins กับ Git

1. New Item → Pipeline, SCM = Git, URL `https://github.com/Thanadonsaer/YGate_git.git`, Script Path `Jenkinsfile`
2. ถ้า repo private เพิ่ม credential (PAT หรือ deploy key) ให้ job
3. ติ๊ก build trigger **"GitHub hook trigger for GITScm polling"**
4. ตั้ง GitHub webhook: Payload URL `https://<jenkins-hostname-ผ่าน-tunnel>/github-webhook/`, content type `application/json`, event `push`
5. ตั้ง Cloudflare Access คลุมทุก path ของ Jenkins UI ยกเว้น `/github-webhook/*` (ต้อง bypass ไม่งั้น GitHub ยิงเข้าไม่ได้)

## 4. Job parameters ([Jenkinsfile](../../Jenkinsfile))

- `PUBLIC_GATEWAY_URL` — ค่าเริ่มต้น `https://ygate.yokogawasolution.com`
- `RELEASES_ROOT` — ค่าเริ่มต้น `D:\YGATE\releases` ต้องตรงกับที่ตั้งไว้ตอน manual deploy
- `ENV_FILE` — ค่าเริ่มต้น `D:\YGATE\ygate.env` ไฟล์นี้ต้องมีอยู่ก่อนแล้ว (สร้างครั้งแรกตาม manual-production.md) — Jenkins ไม่สร้างหรือแก้ไฟล์นี้ให้

กำหนด storage ที่ต้องอยู่ข้าม release ไว้ใน `ENV_FILE` และอย่าวางไว้ใต้ `RELEASES_ROOT`:

```env
PLATFORM_MIDDLEWARE_PATCH_DIR=D:\YGate\data\middleware-patches
PLATFORM_SITE_LOGO_DIR=D:\YGate\data\site-logos
PLATFORM_PLANT_IMAGE_DIR=D:\YGate\data\plant-images
```

สร้าง directory เหล่านี้ล่วงหน้าและให้ account ที่รัน PM2 มีสิทธิ์อ่าน/เขียน เพราะ Jenkins แตก release ใหม่ทุกครั้ง และ path แบบ relative จะชี้ไปยัง release ใหม่
- approver group: `ygate-production-approvers` (ตั้งใน Manage Jenkins → Authorization)

## 5. Pipeline ทำงานอย่างไร

1. **Checkout** — checkout commit ที่ trigger job, เก็บ SHA ไว้ใน `env.RELEASE_SHA` และสร้าง release ID เป็น `jenkins-<BUILD_NUMBER>-<SHA12>`
2. **Validate** (ทุก branch, parallel) — `go test ./...` สามตัว + `npm ci && npm run typecheck` สำหรับ Web
3. **Package** (เฉพาะ `main`) — เรียก `deploy\manual\build-release.ps1` ตัวเดียวกับ manual deploy เพื่อ build Windows binaries + Next.js standalone แล้ว pack เป็น `ygate-jenkins-<build-number>-<sha12>.zip` ใน `dist\jenkins`, archive เป็น Jenkins artifact
4. **Approve Production** (เฉพาะ `main`) — รอ manual approval จาก group `ygate-production-approvers`
5. **Deploy Production** (เฉพาะ `main`) — แตก zip ไปที่ `RELEASES_ROOT\<release-id>` แล้วรัน `start.ps1 -EnvFile <ENV_FILE>` ในโฟลเดอร์นั้น (migrate DB + `pm2 delete` + `pm2 start` + health check เหมือน manual deploy ทุกอย่าง)

Deploy ล้มเหลวถ้า release directory เดิม (`<RELEASES_ROOT>\<sha>`) มีอยู่แล้ว — ป้องกัน build ทับ release ที่กำลังรัน

## 6. Rollback แบบ manual

ไม่มี automatic rollback ในเวอร์ชันนี้ (skip ไว้ก่อนเพราะ manual flow เองก็ไม่มี) ถ้า release ใหม่มีปัญหา:

```powershell
cd D:\YGATE\releases\<previous-sha>
.\start.ps1 -EnvFile D:\YGATE\ygate.env
```

`start.ps1` จะ `pm2 delete` ทุก process ก่อน แล้ว `pm2 start` ใหม่จากโฟลเดอร์ที่รัน — process จึงชี้ไป exe ของ release นั้นจริง ตราบใดที่โฟลเดอร์ release เดิมยังไม่ถูกลบ

> ห้ามเปลี่ยนกลับไปใช้ `pm2 restart` / `pm2 startOrRestart` — ทั้งสองคำสั่ง merge แค่ env เข้า app ที่มีชื่ออยู่แล้ว แต่ยังใช้ `pm_exec_path`/`pm_cwd` เดิมที่จำไว้ตอน start ครั้งแรก ผลคือ deploy ผ่าน health check เขียวหมดแต่ยังรันโค้ด release เก่า

## 7. Security checklist

- Jenkins UI อยู่หลัง Cloudflare Access ทั้งหมด ยกเว้น webhook path
- ห้าม build untrusted pull request ด้วย credential ที่แตะ production ได้
- ใช้ manual approval และ protected branch `main`
- ตั้ง retention ของ `D:\YGATE\releases` และ Jenkins artifacts ไม่ให้ disk เต็ม
- backup `JENKINS_HOME` และ PostgreSQL แยกจากเครื่องหลัก แม้ Jenkins จะอยู่เครื่องเดียวกับ production ก็ตาม
- rotate Git token และ database password เป็นระยะ
