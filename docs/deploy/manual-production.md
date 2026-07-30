# คู่มือ Manual Build และ Deploy Production

คู่มือนี้ใช้สำหรับ Production ที่เป็น **Linux amd64** โดยไม่ต้องติดตั้ง Jenkins:

```text
เครื่อง Build (Linux/WSL2)
  -> ygate-<commit>.tar.gz + .sha256
  -> ลากไฟล์ด้วย WinSCP/SFTP ไป /tmp
  -> sudo ygate-deploy
  -> backup DB, restart, health check และ rollback อัตโนมัติ
```

> ห้ามลาก source code หรือไฟล์ย่อยไปทับ `/opt/ygate/current` โดยตรง ให้ส่งเฉพาะ release archive และ checksum เท่านั้น

## 1. เตรียมเครื่อง Build

ติดตั้ง:

- Git
- Go 1.23 ขึ้นไป
- Node.js 20.9 ขึ้นไป และ npm
- Bash, tar, gzip และ sha256sum
- CA certificates สำหรับดาวน์โหลด dependency

ถ้าใช้ Windows ให้ build ผ่าน **WSL2 หรือ Linux VM** เพราะ Next.js อาจมี native dependency ที่ต้องตรงกับระบบ Production

## 2. Build Release

เลือก commit ที่ผ่านการตรวจสอบแล้ว และ worktree ต้องไม่มีไฟล์แก้ค้าง:

```bash
git fetch --tags origin
git checkout --detach <approved-commit-sha>
git status --short

export NEXT_PUBLIC_GATEWAY_URL=https://scada.example.com
bash deploy/manual/build-release.sh
```

`NEXT_PUBLIC_GATEWAY_URL` ถูกฝังลงใน Web ตอน build จึงต้องเป็น URL จริงของ Production

ไฟล์ผลลัพธ์อยู่ใน `dist/manual/`:

```text
ygate-<40-character-commit-sha>.tar.gz
ygate-<40-character-commit-sha>.tar.gz.sha256
```

หากต้องการวางผลลัพธ์บน Desktop หรือ Shared Folder:

```bash
bash deploy/manual/build-release.sh /mnt/c/Users/<user>/Desktop/ygate-release
```

## 3. เตรียม Production ครั้งแรก

สร้าง user, directory, environment files และ systemd services ตามหัวข้อ Production Services ใน [คู่มือ Jenkins Production](jenkins-production.md)

ลาก `deploy/jenkins/ygate-deploy` จากเครื่อง Build ไปที่ `/tmp/ygate-deploy` หนึ่งครั้ง แล้วติดตั้ง:

```bash
sudo install -m 0755 /tmp/ygate-deploy /usr/local/sbin/ygate-deploy
```

Production ต้องมี Node.js, curl, tar, gzip, sha256sum และ `postgresql-client` สำหรับ `pg_dump` แต่ไม่จำเป็นต้องมี Git, Go หรือ npm

## 4. ลากไฟล์เข้า Production

เปิด WinSCP หรือ SFTP แล้วลาก **ทั้ง 2 ไฟล์** ไปไว้ที่ `/tmp`:

```text
/tmp/ygate-<commit>.tar.gz
/tmp/ygate-<commit>.tar.gz.sha256
```

หรือใช้คำสั่ง:

```bash
scp dist/manual/ygate-<commit-sha>.tar.gz* <deploy-user>@<production-host>:/tmp/
```

## 5. สั่ง Deploy

SSH เข้า Production แล้วรัน:

```bash
sudo /usr/local/sbin/ygate-deploy \
  /tmp/ygate-<commit-sha>.tar.gz \
  <40-character-commit-sha>
```

สคริปต์จะทำสิ่งต่อไปนี้:

1. ตรวจ SHA256 และค่า `VERSION` ใน archive
2. backup PostgreSQL ด้วย `pg_dump`
3. แตก release ใหม่ใน `/opt/ygate/releases/`
4. สลับ symlink `/opt/ygate/current`
5. restart Platform API, API Gateway และ Web
6. ตรวจ health endpoint
7. rollback release เดิมอัตโนมัติถ้า health check ไม่ผ่าน

## 6. ตรวจสอบหลัง Deploy

```bash
readlink -f /opt/ygate/current

sudo systemctl status ygate-platform-api ygate-api-gateway ygate-web --no-pager

curl --fail http://127.0.0.1:44441/readyz
curl --fail http://127.0.0.1:44440/gateway/healthz
curl --fail http://127.0.0.1:44440/readyz
curl --fail http://127.0.0.1:8080/
```

ทดสอบจาก URL ภายนอกเพิ่ม:

- Login และเปิดหน้า Web
- เปิด OpenAPI
- ทดสอบ WebSocket
- ส่ง telemetry จาก Middleware
- เปิด Plant แล้วตรวจ latest telemetry ของ Device

ดู log เมื่อมีปัญหา:

```bash
sudo journalctl -u ygate-platform-api -u ygate-api-gateway -u ygate-web -n 200 --no-pager
```

## 7. Rollback ด้วยมือ

ดู release ก่อนหน้า:

```bash
ls -la /opt/ygate/releases
readlink -f /opt/ygate/current
```

สลับกลับแล้ว restart:

```bash
sudo ln -sfn /opt/ygate/releases/<previous-commit> /opt/ygate/current
sudo systemctl restart ygate-platform-api ygate-api-gateway ygate-web
```

จากนั้นตรวจ health endpoint ซ้ำ

> Database migration ควรเป็นแบบ backward-compatible เพราะการ rollback application ไม่ได้ย้อน schema อัตโนมัติ และควรกำหนด retention สำหรับ `/opt/ygate/releases` กับ `/opt/ygate/backups`

## กรณี Production เป็น Windows

สคริปต์ชุดนี้รองรับ Linux เท่านั้น หาก Production เป็น Windows ต้องเปลี่ยนเป็น Windows Service/NSSM, PowerShell deploy script และ build Go ด้วย `GOOS=windows` แยกต่างหาก
