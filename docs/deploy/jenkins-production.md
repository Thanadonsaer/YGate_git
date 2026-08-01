# คู่มือ CI/CD: Git → Jenkins → Production

คู่มือนี้ใช้ Jenkins LTS บน Linux build agent และ deploy immutable artifact ผ่าน SSH ไปยัง production Linux amd64 ไม่ build ทับ directory ที่กำลังรัน และไม่เก็บ production secrets ใน Git หรือ Jenkins artifact

```text
Git push / merge main
        ↓ webhook
Jenkins checkout → test → build → package
        ↓ manual approval
SSH upload → DB backup → deploy ทีละ service
        ↓
health gate ผ่าน = release สำเร็จ
health gate ไม่ผ่าน = rollback symlink
```

> ถ้า production เป็น Windows หรือ ARM64 ต้องเปลี่ยน service manager, remote deploy script และ `GOOS/GOARCH` ใน `Jenkinsfile`

## 1. เตรียมเครื่อง

แนะนำให้แยก Jenkins controller/build agent ออกจาก production เครื่อง Jenkins ต้องมี:

- Jenkins LTS และ Java 21
- Git, `curl`, `tar`, `gzip`, `openssh-client` และ CA certificates
- Go 1.23 ขึ้นไป
- Node.js 20.9 ขึ้นไปพร้อม npm
- network access ไปยัง Git server, Go module proxy/npm registry และ production SSH port
- พื้นที่อย่างน้อย 50 GB สำหรับ small team ตามคำแนะนำ Jenkins

ติดตั้ง Jenkins ตาม [Jenkins Linux installation](https://www.jenkins.io/doc/book/installing/linux/) และใช้ Java version ตาม [Java support policy](https://www.jenkins.io/doc/book/platform-information/support-policy-java/) จากนั้นเปิด Jenkins ผ่าน HTTPS reverse proxy ห้ามเปิด Jenkins port ต่อ Internet โดยตรง

Production ต้องมี:

- Linux amd64, systemd, Node.js 20.9 ขึ้นไป
- `curl`, `tar`, `gzip`, `openssh-server`, `postgresql-client` และ CA certificates
- PostgreSQL ที่ production เข้าถึงได้
- TLS reverse proxy ตามหัวข้อ Production ใน root `README.md`
- user `ygate` สำหรับรัน services และ user `ygate-deploy` สำหรับรับ artifact

Go runtime และ Git ไม่จำเป็นบน production เพราะ Jenkins ส่ง native binaries และ Web standalone artifact มาให้

## 2. ติดตั้ง Jenkins plugins

ไปที่ **Manage Jenkins → Plugins** แล้วติดตั้ง:

- Pipeline
- Git
- Credentials Binding

`Jenkinsfile` ใช้ `withCredentials` กับ SSH private key โดยตรง จึงไม่ต้องติดตั้ง SSH Agent plugin ถ้าจะเปลี่ยนไปใช้ `sshagent` agent ต้องมี executable `ssh-agent` ตาม [SSH Agent plugin requirements](https://plugins.jenkins.io/ssh-agent/)

Jenkins แนะนำให้เก็บ pipeline definition เป็น `Jenkinsfile` ใน source control เพื่อให้ review และ audit ได้ ดู [Using a Jenkinsfile](https://www.jenkins.io/doc/book/pipeline/jenkinsfile/)

## 3. เตรียม Production services

สร้าง users และ directories:

```bash
sudo useradd --system --home /opt/ygate --shell /usr/sbin/nologin ygate
sudo useradd --create-home --shell /bin/bash ygate-deploy
sudo install -d -o ygate -g ygate /opt/ygate/releases
sudo install -d -m 0750 -o root -g ygate /etc/ygate /var/backups/ygate
sudo install -m 0755 deploy/jenkins/ygate-deploy /usr/local/sbin/ygate-deploy
```

สร้าง `/etc/ygate/ygate.env` โดยใช้ production values จาก root `README.md` และกำหนดเพิ่ม:

```dotenv
PLATFORM_OPENAPI_FILE=/opt/ygate/current/platform-api.yaml
HOSTNAME=127.0.0.1
PORT=8080
```

ต้องมี `AUTH_DATABASE_URL`, `AUTH_HTTP_ADDR`, `AUTH_COOKIE_SECURE` และ `GATEWAY_AUTH_SERVICE_URL` (ชี้ไปที่ `AUTH_HTTP_ADDR`) ตามที่ระบุใน root `README.md` ด้วย ไม่เช่นนั้น `ygate-auth-service` จะไม่ทำงานและ gateway จะ 502 ทุก login/admin request

ไฟล์ต้องเป็น shell-compatible `KEY=value` และจำกัดสิทธิ์:

```bash
sudo chown root:ygate /etc/ygate/ygate.env
sudo chmod 0640 /etc/ygate/ygate.env
```

สร้าง systemd units ต่อไปนี้

`/etc/systemd/system/ygate-platform-api.service`:

```ini
[Unit]
Description=YGATE Platform API
After=network-online.target
Wants=network-online.target

[Service]
User=ygate
Group=ygate
WorkingDirectory=/opt/ygate/current
EnvironmentFile=/etc/ygate/ygate.env
ExecStart=/opt/ygate/current/bin/platform-api
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

`/etc/systemd/system/ygate-auth-service.service`:

```ini
[Unit]
Description=YGATE Auth Service
After=network-online.target
Wants=network-online.target

[Service]
User=ygate
Group=ygate
WorkingDirectory=/opt/ygate/current
EnvironmentFile=/etc/ygate/ygate.env
ExecStart=/opt/ygate/current/bin/auth-service
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

`/etc/systemd/system/ygate-api-gateway.service`:

```ini
[Unit]
Description=YGATE API Gateway
After=ygate-platform-api.service ygate-auth-service.service

[Service]
User=ygate
Group=ygate
WorkingDirectory=/opt/ygate/current
EnvironmentFile=/etc/ygate/ygate.env
ExecStart=/opt/ygate/current/bin/api-gateway
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

`/etc/systemd/system/ygate-web.service`:

```ini
[Unit]
Description=YGATE Web
After=ygate-api-gateway.service

[Service]
User=ygate
Group=ygate
WorkingDirectory=/opt/ygate/current/web
EnvironmentFile=/etc/ygate/ygate.env
ExecStart=/usr/bin/node /opt/ygate/current/web/server.js
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

เปิด services หลัง first release ถูกวางแล้ว:

```bash
sudo systemctl daemon-reload
sudo systemctl enable ygate-platform-api ygate-auth-service ygate-api-gateway ygate-web
```

อนุญาตให้ deploy user รันเฉพาะ reviewed deploy script ผ่าน `/etc/sudoers.d/ygate-deploy`:

```sudoers
ygate-deploy ALL=(root) NOPASSWD: /usr/local/sbin/ygate-deploy
```

ตรวจด้วย `sudo visudo -cf /etc/sudoers.d/ygate-deploy`

## 4. ตั้งค่า SSH

1. สร้าง SSH key สำหรับ Jenkins โดยเฉพาะ ห้าม reuse personal key
2. ใส่ public key ใน `/home/ygate-deploy/.ssh/authorized_keys`
3. เก็บ private key ใน Jenkins: **Manage Jenkins → Credentials → System → Global credentials → Add Credentials**
4. เลือกชนิด **SSH Username with private key**, username `ygate-deploy`, ID `ygate-production-ssh`
5. รัน `ssh-keyscan <production-host>` จากเครื่องที่เชื่อถือได้ ตรวจ fingerprint กับ production admin แล้วเก็บผลเป็น Jenkins **Secret file** ID `ygate-production-known-hosts`

ห้ามใช้ `StrictHostKeyChecking=no` และห้ามสร้าง `known_hosts` ใหม่ทุก build

## 5. เชื่อม Jenkins กับ Git

1. Push `Jenkinsfile` และไฟล์ deploy ทั้งหมดเข้า repository
2. ถ้า repository เป็น private ให้เพิ่ม read-only deploy key/token ใน Jenkins Credentials
3. สร้าง **Multibranch Pipeline** แล้วเลือก repository URL และ credential
4. ตั้ง Script Path เป็น `Jenkinsfile`
5. จำกัด production deploy ที่ protected branch `main`; pull request ทำได้เฉพาะ Validate
6. ตั้ง webhook จาก Git server มายัง Jenkins เพื่อ trigger scan/build

Git plugin รองรับ HTTPS credential แบบ username/password และ SSH แบบ private-key credential และแนะนำ webhook เพื่อลด polling delay ดู [Jenkins Git plugin](https://plugins.jenkins.io/git)

ถ้าใช้ GitHub webhook ปกติคือ `https://ci.example.com/github-webhook/` ถ้าเป็น Git server ทั่วไป ใช้ authenticated `post-receive` hook เรียก Jenkins Git `notifyCommit` ตามเอกสาร plugin

## 6. ตั้งค่า Jenkins job

แก้ค่าเริ่มต้นใน `Jenkinsfile` หรือกรอก parameters ตอนรัน:

- `PUBLIC_GATEWAY_URL`: เช่น `https://scada.example.com`
- `PRODUCTION_HOST`: DNS/IP ที่ Jenkins SSH ถึงได้
- agent label: `ygate-linux-build`
- approver group: `ygate-production-approvers`

Pipeline ทำงานดังนี้:

1. Checkout exact commit SHA ที่ trigger job
2. รัน Go tests สอง services และ Web typecheck
3. Build Linux amd64 binaries และ Next.js standalone artifact
4. สร้าง `tar.gz`, SHA-256 checksum และ archive ใน Jenkins
5. รอ manual production approval เฉพาะ branch `main`
6. Upload artifact และ SHA-256 checksum ผ่าน SSH แล้วตรวจ checksum ก่อนแตกไฟล์
7. Production script backup PostgreSQL, สลับ release symlink, restart ทีละ service และตรวจ health
8. ถ้า health gate fail จะสลับกลับ previous release และ restart ทุก service

## 7. First deploy

Deploy script ต้องมี previous release สำหรับ automatic rollback ดังนั้น first deploy ควรทดสอบบน staging ก่อน Production และตรวจว่า:

```bash
curl --fail http://127.0.0.1:44441/readyz
curl --fail http://127.0.0.1:44442/readyz
curl --fail http://127.0.0.1:44440/gateway/healthz
curl --fail http://127.0.0.1:44440/readyz
curl --fail http://127.0.0.1:8080/
```

จากนั้นทดสอบผ่าน public HTTPS: login, OpenAPI, WebSocket และ test Middleware ingestion

## 8. Security และ Operations checklist

- Backup `JENKINS_HOME`, Jenkins credentials encryption keys และ PostgreSQL แยกจากเครื่องหลัก
- ให้ Jenkins build untrusted pull request โดยไม่มี production credentials
- ใช้ manual approval และ protected `main`
- จำกัด SSH/firewall ให้ Jenkins ติดต่อ production ได้เฉพาะ port ที่จำเป็น
- rotate Git token, SSH deploy key, database password และ TLS certificates
- ตั้ง retention ของ `/opt/ygate/releases`, `/var/backups/ygate` และ Jenkins artifacts
- monitor `/readyz`, `/gateway/healthz`, systemd logs, PostgreSQL และ disk usage
- อย่าย้อน `schema_migrations`; migrations เป็น forward-only ต้อง rollback เฉพาะ application artifact ที่ยังรองรับ schema ใหม่

## 9. คำสั่งตรวจสอบและ rollback แบบ manual

```bash
sudo systemctl status ygate-platform-api ygate-auth-service ygate-api-gateway ygate-web
sudo journalctl -u ygate-platform-api -u ygate-auth-service -u ygate-api-gateway -u ygate-web --since '15 minutes ago'
readlink -f /opt/ygate/current
ls -1 /opt/ygate/releases
```

Rollback ไป release เดิม:

```bash
sudo ln -sfn /opt/ygate/releases/<previous-commit-sha> /opt/ygate/current.rollback
sudo mv -Tf /opt/ygate/current.rollback /opt/ygate/current
sudo systemctl restart ygate-platform-api ygate-auth-service ygate-api-gateway ygate-web
```
