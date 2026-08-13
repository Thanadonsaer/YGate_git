# คู่มือ Manual Build และ Deploy บน Windows + PM2

Production ใช้ Windows amd64 และ Cloudflare Tunnel โดยเปิด service เฉพาะ localhost:

```text
Cloudflare Tunnel
  app hostname -> 127.0.0.1:8080  (Web)
  API hostname -> 127.0.0.1:44440 (Gateway)
                                   -> 127.0.0.1:44441 (Platform API)
```

## Build และ Pack

เครื่อง Build ต้องมี Git, Go 1.23+, Node.js 20.9+ และ npm จากนั้นเปิด PowerShell:

```powershell
.\deploy\manual\build-release.ps1
```

ค่า Gateway เริ่มต้นคือ `https://ygate.yokogawasolution.com` เปลี่ยนได้ด้วย:

```powershell
.\deploy\manual\build-release.ps1 -PublicGatewayUrl "https://api.example.com"
```

ผลลัพธ์อยู่ใน `dist\manual`:

```text
ygate-<release>.zip
ygate-<release>.zip.sha256
```

หาก worktree มีไฟล์ที่ยังไม่ commit ชื่อ release จะมี `-dirty-<timestamp>` เพื่อไม่ให้สับสนกับ commit ปกติ

## ลากไฟล์และรัน

1. ลาก ZIP ไปเครื่อง Production เช่น `D:\YGATE\packages`
2. ตรวจ checksum แล้วแตกลง release directory ใหม่ ห้ามแตกทับ release ที่กำลังรันเพราะ Windows ล็อกไฟล์ `.exe`:

```powershell
$release = "<release>"
$zip = "D:\YGATE\packages\ygate-$release.zip"
$target = "D:\YGATE\releases\$release"
(Get-FileHash $zip -Algorithm SHA256).Hash
Expand-Archive $zip $target
Set-Location $target
```

## สร้าง PostgreSQL ครั้งแรก

ติดตั้ง PostgreSQL บน Windows แยกต่างหาก จากนั้นเปิด SQL Shell (`psql`) ด้วย user `postgres`:

```powershell
psql -U postgres -d postgres
```

สร้าง application user และ database:

```sql
CREATE ROLE ygate_app WITH LOGIN PASSWORD '<strong-password>';
CREATE DATABASE ygate_db OWNER ygate_app;
```

ไม่ต้อง copy หรือรันไฟล์ SQL จาก repository เพราะ migrations ถูกฝังใน `platform-api.exe` และทำงานอัตโนมัติตอน start

3. รันครั้งแรกเพื่อสร้าง environment file กลางที่ไม่ถูกทับตอน deploy:

```powershell
.\start.ps1 -EnvFile "D:\YGATE\ygate.env"
```

4. แก้ PostgreSQL credential และ hostname ใน `ygate.env` แล้วรันซ้ำ:

```powershell
.\start.ps1 -EnvFile "D:\YGATE\ygate.env"
pm2 status
pm2 logs
```

`start.ps1` จะ start/restart Platform API, Gateway และ Web ผ่าน PM2 จากนั้นบันทึก process list และตรวจ health endpoints

ค่า database ใน `D:\YGATE\ygate.env`:

```dotenv
PLATFORM_DATABASE_URL=postgresql://ygate_app:<URL-encoded-password>@127.0.0.1:5432/ygate_db
PLATFORM_PUBLIC_BASE_URL=https://ygate-api.yokogawasolution.com
```

ใช้ API hostname โดยตรงสำหรับ `PLATFORM_PUBLIC_BASE_URL` เพื่อให้ Middleware ดาวน์โหลด patch โดยไม่ผ่าน Web service.

หลัง services พร้อม ให้สร้างผู้ดูแลระบบคนแรกหนึ่งครั้ง:

```powershell
.\bootstrap-admin.ps1 `
  -EnvFile "D:\YGATE\ygate.env" `
  -Email "admin@example.com" `
  -DisplayName "System Admin" `
  -OrganizationCode "YGATE" `
  -OrganizationName "YGATE"
```

สคริปต์จะถาม password แบบซ่อน แล้วสร้าง organization, user และ role `System Admin`

## Cloudflare Tunnel

กำหนด Public Hostname อย่างน้อยสองรายการ:

```text
ygate.yokogawasolution.com     -> http://127.0.0.1:8080
ygate-api.yokogawasolution.com -> http://127.0.0.1:44440
```

ไม่ expose port `44441` และไม่ต้องเปิด inbound port `8080`, `44440`, `44441` ที่ Windows Firewall

> PM2 บน Windows ต้องมี startup mechanism เพิ่มเพื่อกลับมาหลัง reboot; Cloudflare Tunnel ควรรันเป็น Windows Service
