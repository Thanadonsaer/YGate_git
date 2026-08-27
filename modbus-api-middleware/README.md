# CHPP Modbus API Middleware

Go middleware สำหรับอ่าน Modbus TCP ตามตาราง configuration แล้วส่ง KPI ไปยัง Y-Gate Solar API โดย Windows ใช้ SQLite และ RUT906/MIPS ใช้ไฟล์ `middleware.store` แบบ atomic

### RUT906 / MIPS

RUT906 ใช้สถาปัตยกรรม MIPS ซึ่งไม่รองรับ SQLite dependency เดิม จึงใช้ backend ไฟล์ที่พฤติกรรมและ config/API เหมือนเดิม แต่ไม่สามารถเปิดไฟล์ `middleware.db` โดยตรงได้

1. เตรียม `license-keys.env` ให้มี `CHPP_LICENSE_PUBLIC_KEY` แล้วรัน `build-all.bat` เพื่อให้ public key ถูกฝังใน binary (ห้ามใช้ `go build` เปล่า ๆ เพราะจะได้ binary ที่ activate license ไม่ได้) หรือ build เองด้วย `GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0 go build -trimpath -ldflags "-X main.licensePublicKey=<PUBLIC_KEY>" -o build/linux/build/middleware-linux-mipsle ./cmd/middleware`
2. คัดลอก `middleware-linux-mipsle`, `deploy/`, `license.json` และถ้ามี `middleware.store` ไปไว้โฟลเดอร์เดียวกันบน RUT906
3. บน RUT906 ให้ SSH เป็น `root` (RutOS ไม่มี `sudo`) แล้วรัน `chmod +x deploy/manage-service.sh deploy/install-systemd.sh` และ `./deploy/manage-service.sh`
4. เลือก `1) Install / Update service` จากนั้น activate license และเลือก `3) Start Service`

บน RUT906 installer จะใช้ OpenWrt `procd` ผ่าน `/etc/init.d/chpp-middleware` และเก็บ runtime ที่โฟลเดอร์ USB เดียวกับชุด deploy เช่น `/tmp/sdb1/linux/runtime/middleware.store` เพราะ `/opt` ของ RutOS อาจเป็น read-only ส่วน Linux ที่มี systemd จะใช้ `/opt/chpp-middleware` และ Windows ยังคงใช้ `middleware.db` เดิม หากต้องย้าย config ระหว่าง Windows กับ RUT906 ให้ใช้เมนู export/import config ของ Web UI ไม่ต้องคัดลอก SQLite database

RUT906 ที่ติดตั้ง service ผ่าน `procd` รองรับ remote Stage/Apply/Rollback/Restart ด้วย binary รุ่นที่ build ใหม่แล้ว โดย patch ต้องระบุ architecture เป็น `mipsle` ให้ตรงกับ `runtime.GOARCH` ของ Go (ชื่อไฟล์ binary ยังคงเป็น `middleware-linux-mipsle`)

## Pages

- Address Manager: `http://127.0.0.1:8081/`
- Gateway API Config: `http://127.0.0.1:8081/config`
- API Send Logs: `http://127.0.0.1:8081/logs`
- Brand Detail: กดปุ่ม `Device` ใน Brand list
- Export Template: กด `Export Excel Template` หรือเปิด `/template/device-set-address.csv`
- Import Template: กด `Import Device Set + Address` หรือเปิด `/import-template`

## Import Device Set + Address

1. Export template CSV
2. เปิดไฟล์ด้วย Excel แล้วกรอก/แก้รายการ Address
3. Save กลับเป็น `.csv`
4. เข้า `/import-template` แล้วเลือกไฟล์เพื่อ import

ระบบจะ group ตาม `brand_name + dev_type + dev_model` แล้วสร้าง/อัปเดต Device Set พร้อม Address ให้เอง ถ้า import Device Set เดิมซ้ำ ระบบจะ replace address list ของ Device Set นั้น

คอลัมน์ที่รองรับ:

```text
brand_name, dev_type, dev_model, address_fc, register, description, factor, data_type, remark
```

CSV เดิมด้านบนยังใช้ได้เหมือนเดิม ส่วน template ใหม่เพิ่ม configuration แบบเต็ม:

```text
address_mode, byte_order, word_order, max_block_size, canonical_key, source_tag,
offset, address_length, address_word_order, source_unit, canonical_unit, enabled
```

## Configuration model

```text
Gateway Config -> API endpoint/api key/send interval

Brand 1 -> * Device Set 1 -> * Address
                    |
                    +---- * Connection
```

| Table | Fields |
|---|---|
| `brands` | `brand_id`, `brand_name`, `brand_description`, `brand_dev_set_list` |
| `device_sets` | `dev_set`, `dev_type`, `dev_model`, `address_mode`, `byte_order`, `word_order`, `max_block_size` |
| `addresses` | FC, register, data type/length, factor/offset, per-address word order, units, enabled |
| `connections` | host, port, unit ID, device set, poll/publish interval |
| `gateway_config` | `gateway_id`, `endpoint`, `api_key`, `send_interval_seconds` |
| `outbox_events` | API send log/status/retry state |

### Modbus connection settings

- **Host / Port** ระบุปลายทาง TCP เช่น `192.168.1.43:502`
- **Unit ID** เลือกอุปกรณ์ปลายทางภายใน gateway เดียวกัน
- **Function Code (FC)** เลือกพื้นที่ข้อมูล เช่น FC03 Holding Registers หรือ FC04 Input Registers
- **Register** คือเลข address ที่ตีความตาม Address Mode
- **Byte Order** กำหนดลำดับ byte ในแต่ละ 16-bit register
- **Word Order** กำหนดลำดับ word ของค่า 32/64-bit และ override ต่อ Address ได้

Address Mode ที่รองรับ:

| Mode | การแปลง |
|---|---|
| `ZERO_BASED` | ส่ง register ตามที่กรอก (รองรับการ normalize full notation แบบเดิมตอนบันทึกเพื่อ backward compatibility) |
| `ONE_BASED` | ลบ 1 ก่อนส่ง เช่น `2081 -> 2080` |
| `FULL_NOTATION` | `32080 -> FC03/2080`, `40196 -> FC04/196` |
| `REGISTER_30001` | ลบ 30001 ก่อนส่ง เช่น `30057 -> 56` โดยคง FC ตามที่กรอก |
| `REGISTER_40001` | ลบ 40001 ก่อนส่ง เช่น `40084 -> 83` โดยคง FC ตามที่กรอก |
| `VENDOR_RAW` | ส่ง address ตามที่กรอกโดยไม่แปลง |
| `SMA` | เหมือน vendor raw สำหรับ SMA profile |

Alias ที่รับได้: `RAW`/`DIRECT` -> `VENDOR_RAW`, `OFFSET_0` -> `ZERO_BASED`, `OFFSET_1` -> `ONE_BASED`, `HOLDING_40001` -> `REGISTER_40001`, `INPUT_30001` -> `REGISTER_30001`.

คำแนะนำจาก `Address.xlsx` สำหรับ inverter ชุดนี้:

| Brand/Model | Address Mode | FC | Byte/Word | Address ที่เปิดใช้ |
|---|---|---:|---|---|
| ABB PVS100 | `VENDOR_RAW` | 03 | `BIG_ENDIAN` / `HIGH_LOW` | `40084` active_power `SHORT` factor `0.01`; `40108` inverter_state `USHORT`; `40233` active_power_adjustment `USHORT` factor `0.1` |
| ABB PVS50 | `VENDOR_RAW` | 03 | `BIG_ENDIAN` / `HIGH_LOW` | เหมือน PVS100 |
| Huawei SUN2000 | `VENDOR_RAW` | 03 | `BIG_ENDIAN` / `HIGH_LOW` | `32080` active_power `INT32` factor `0.001`; `32089` inverter_state `USHORT`; `32114` day_cap `UINT32` factor `0.01`; `35302` active_power_adjustment `SHORT` factor `0.1` |

หมายเหตุ: `FULL_NOTATION` ยังมีไว้สำหรับงานเดิมที่ต้องแปลง `32080 -> FC03/2080` หรือ `40196 -> FC04/196` แต่สำหรับ ABB PVS และ Huawei SUN2000 ในไฟล์นี้แนะนำให้ใช้ `VENDOR_RAW` เพื่อส่งเลข register ตาม vendor document ตรง ๆ. ถ้าทดสอบหน้างานแล้วค่า 32-bit ของ Huawei กลับ word order ให้เปลี่ยนเฉพาะ address นั้นเป็น `SW_INT` / `SW_UINT` หรือ `address_word_order=LOW_HIGH`.
## Modbus read path

```text
Connection
-> Device Set
-> Address rows
-> resolve address mode และ group เฉพาะ register ที่ FC เดียวกัน/ต่อเนื่องกัน
-> read Modbus TCP by unit_id / slave_id
-> ถ้า block ล้มเหลว fallback อ่านแยกราย Address
-> decode data type
-> raw registerAddressMap (`functionCode:registerAddress`)
-> persistent outbox
-> delivery worker sends to Gateway Config endpoint by send_interval_seconds
```

Data type ที่รองรับ: `SHORT`, `USHORT`, `INT32`, `UINT32`, `LONG`, `ULONG`, `UINT64`, `FLOAT`, `SW_INT`, `SW_UINT`, `SW_FLOAT` และ `S32`, `U32`, `U64` ซึ่งตรวจ sentinel/NaN ของ SMA

## SMA Sunny Central

ในหน้าสร้าง Connection กด `+ เพิ่ม/เลือก SMA Profile` เพื่อสร้าง Device Set สำหรับ Sunny Central ผ่าน SC-COM จากนั้นผู้ใช้กรอก Host, Port และ Unit ID ของอุปกรณ์

SMA profile ใช้ FC03, big-endian/high-word-first และส่ง register `30057..34113` ตรงไปยังอุปกรณ์โดยไม่ normalize แบบ Huawei รองรับค่า SMA `S32`, `U32`, `U64`, scaling และข้ามค่า NaN ของ SMA อัตโนมัติ หากบาง address ไม่มีใน inverter รุ่นนั้น ระบบจะเก็บค่าจาก address อื่นที่อ่านได้ต่อไป

ตัวอย่าง Modbus Poll ที่ตรงกับ `sma.py`:

```text
Connection:    Modbus TCP/IP
Host:          192.168.1.43
Port:          502
Slave/Unit ID: 43
Function:      03 Read Holding Registers
Address:       30057
Quantity:      2
Display:       UINT32, Big-endian, High-word-first
```

สำหรับ SMA เลข `30057` เป็น vendor raw protocol address จึงต้องส่ง start address `30057` จริง ไม่ใช่ `57`

## Run

เปิดด้วยไฟล์:

```powershell
.\run-middleware.bat
```

หรือสั่งเอง:

```powershell
cd D:\PROJECT_2026\CHPP\modbus-api-middleware

.\build\middleware-v0.2.6-windows-amd64.exe `
  -run `
  -db middleware.db `
  -listen 0.0.0.0:8081
```

ถ้าต้องการ seed config ผ่าน command line ยังใช้ได้:

```powershell
.\build\middleware-v0.2.6-windows-amd64.exe `
  -run `
  -gateway-id MOXA-VT1-01 `
  -db middleware.db `
  -listen 0.0.0.0:8081 `
  -endpoint http://192.168.1.108:44440/api/v2/ingestion/register-readings `
  -api-key YOUR_API_KEY
```

เพิ่ม `-configure-only` เมื่อต้องการบันทึกค่าแล้วออก โดยไม่ poll อุปกรณ์หรือเปิด web server

## Version

```bash
./build/middleware-v0.2.6-linux-amd64 -version
# หรือ Windows
.\build\middleware-v0.2.6-windows-amd64.exe -version
```

## Configuration API

```text
GET/POST /api/gateway-config
GET      /api/delivery-logs
POST     /api/import-device-set-address
GET/POST /api/brands
GET/POST /api/device-sets
GET/POST /api/connections
POST     /api/read-now/{connectionId}
```

## Verify and build

Build ตอนนี้สร้าง Windows exe และ Linux binaries สำหรับ `amd64`, `armv7` และ `arm64`

```bat
build-all.bat
```

ผลลัพธ์หลัก:

```text
build\middleware-v0.2.6-windows-amd64.exe
build\linux\build\middleware-linux-amd64
build\linux\build\middleware-linux-armv7
build\linux\build\middleware-linux-arm64
build\linux\deploy\*.sh
build\patches\chpp-middleware-v0.2.6-windows-amd64-update.zip
build\patches\chpp-middleware-v0.2.6-linux-amd64-update.zip
build\patches\chpp-middleware-v0.2.6-linux-armv7-update.zip
build\patches\chpp-middleware-v0.2.6-linux-arm64-update.zip
```


`build-all.bat` จะ clean output เก่าแบบ best-effort, รัน `go test ./...`, `go vet ./...` แล้ว build Windows amd64 + Linux amd64/armv7/arm64 ถ้ามี process เก่าล็อกไฟล์ใน `build` ให้ปิดโปรแกรม/service เก่าก่อนแล้วรัน build อีกครั้ง

## Modbus TCP Simulator

Simulator แยกจาก middleware exe สำหรับทดสอบ read/discover:

```powershell
go run ./simulator-modbus -listen 127.0.0.1:1502 -unit 1 -profile huawei
```

ตั้ง connection ใน middleware เป็น host `127.0.0.1`, port `1502`, unit id `1`. เปลี่ยน profile ได้เป็น `huawei`, `abb`, หรือ `generic`.
## License + Windows Service ผ่าน EXE CLI

ดับเบิลคลิก EXE หรือเปิด EXE โดยไม่มี argument เพื่อแสดงเมนู Service Manager สำหรับ Install/Update, Status, Start, Stop, Restart, Uninstall, เปิด Web UI และจัดการ license โดยไม่ต้องพิมพ์ command เอง

เมนูจะไม่เปิด SQLite, polling หรือ port `8081` จนกว่าจะเลือก Install/Start Service ให้เปิด EXE แบบ Run as Administrator เมื่อต้องการเปลี่ยนสถานะ Windows Service

```powershell
$exe = (Resolve-Path ".\build\middleware-v0.2.6-windows-amd64.exe").Path
$db = Join-Path (Split-Path $exe) "middleware.db"
$license = Join-Path (Split-Path $exe) "license.json"
```

เตรียม license ก่อนติดตั้ง Service:

```powershell
& $exe -machine-id
& $exe -activate-license "LICENSE_TOKEN" -license-file $license
& $exe -license-status -license-file $license
```

ติดตั้งหรืออัปเดต Service ให้เรียบร้อยแล้ว start อัตโนมัติ:

```powershell
& $exe -service install -db $db -listen 0.0.0.0:8081 -license-file $license -cleanup-retention-days 30
```

จัดการ Service ผ่าน EXE:

```powershell
& $exe -service status
& $exe -service start
& $exe -service stop
& $exe -service restart
& $exe -service uninstall
```

คำสั่ง `uninstall` ลบเฉพาะ Windows Service ไม่ลบ `middleware.db` หรือ `license.json` ส่วน console mode สำหรับพัฒนา/ทดสอบต้องระบุ `-run` ชัดเจน:

```powershell
& $exe -run -gui -db $db -listen 127.0.0.1:8081
```
## License key ตอน build

สร้าง keypair ครั้งแรกผ่าน wizard:

```bat
Licensegen.bat
```

หรือใช้ command ตรง:

```powershell
go run .\cmd\licensegen -generate-keypair
```


เก็บ `CHPP_LICENSE_PRIVATE_KEY` ไว้นอก repo และไม่ส่งให้ลูกค้า ส่วน public key ใส่ไว้ใน `license-keys.env`:

```powershell
copy .\license-keys.env.example .\license-keys.env
notepad .\license-keys.env
```

ตัวอย่าง:

```env
CHPP_LICENSE_KEY_NAME=DEFAULT
CHPP_LICENSE_PUBLIC_KEY_DEFAULT=<PUBLIC_KEY>
```

ตอน build ระบบจะฝัง public key เข้า exe อัตโนมัติ ถ้าไม่เจอ key จะหยุด build

ออก token ให้ลูกค้า:

```powershell
$env:CHPP_LICENSE_PRIVATE_KEY="<PRIVATE_KEY>"
go run .\cmd\licensegen -customer "Customer Name" -machine-id "<MACHINE_ID>" -expires 2027-12-31
```

## Deploy บน Linux (systemd)

ใช้ package จาก `build/linux` ซึ่งประกอบด้วย binary, script deploy และ systemd unit

```text
build/linux/
├── build/middleware-linux-amd64
├── build/middleware-linux-armv7
├── build/middleware-linux-arm64
├── deploy/chpp-middleware.service
├── deploy/install-systemd.sh
├── deploy/manage-service.sh
└── README.md
```

### 1. เตรียมไฟล์บนเครื่อง build

หลังจากรัน `build-all.bat` ให้คัดลอก `build/linux` ไปยัง Linux server เช่น:

```bash
scp -r build/linux ygate@middleware-host:/tmp/chpp-middleware
```

ถ้ามี license แล้ว ให้วาง `license.json` ไว้ที่ package root เดียวกับโฟลเดอร์ `deploy`:

```text
/tmp/chpp-middleware/license.json
/tmp/chpp-middleware/build/middleware-linux-<architecture>
/tmp/chpp-middleware/deploy/manage-service.sh
```

### 2. Activate license และติดตั้ง service

เข้า server แล้วใช้เมนูเดียวกับ Windows:

```bash
cd /tmp/chpp-middleware
chmod +x build/middleware-linux-* deploy/*.sh
./deploy/manage-service.sh
```

เมนูมีรายการเดียวกับ Windows:

```text
1) Install / Update service
2) Service Status
3) Start Service
4) Stop Service
5) Restart Service
6) Uninstall Service
7) Open Web UI
8) Show Machine ID
9) Activate License
0) Exit
```

สำหรับ installation ใหม่ ให้เลือก `8` เพื่อส่ง Machine ID ให้ผู้ดูแลออก license จากนั้นเลือก `9` แล้ววาง license token ระบบจะบันทึก `license.json` ไว้ใน package และเลือก `1` เพื่อติดตั้ง/อัปเดต service

เมนูต้องใช้สิทธิ์ `sudo` สำหรับติดตั้งและจัดการ systemd service โดยไม่ต้องเปิด shell ค้างไว้หลังติดตั้ง

### 3. ตั้งค่า Gateway

แก้ค่าที่เครื่องปลายทาง:

```bash
sudo nano /etc/chpp-middleware.env
sudo systemctl restart chpp-middleware
```

ค่าหลักที่ใช้:

```env
GATEWAY_ID=MOXA-VT1-01
DB_PATH=/opt/chpp-middleware/middleware.db
LISTEN_ADDR=0.0.0.0:8081
CLEANUP_RETENTION_DAYS=30
```

`middleware.db` และ `license.json` จะถูกเก็บไว้ที่ `/opt/chpp-middleware` และจะไม่ถูกลบเมื่อเลือก `Uninstall Service`

### 4. ตรวจสอบ service และ logs

```bash
sudo systemctl status chpp-middleware --no-pager
sudo systemctl is-enabled chpp-middleware
sudo journalctl -u chpp-middleware -n 100 --no-pager
sudo journalctl -u chpp-middleware -f
```

เปิด Web UI จากเครื่องที่เข้าถึง server ได้ที่ `http://<server-ip>:8081/` หรือเลือกเมนู `7` เมื่อมี desktop browser บน Linux server

### Direct commands

ถ้าไม่ใช้เมนู สามารถใช้คำสั่งตรงได้:

```bash
BIN=/opt/chpp-middleware/middleware
LICENSE=/opt/chpp-middleware/license.json

"$BIN" -machine-id
"$BIN" -activate-license "LICENSE_TOKEN" -license-file "$LICENSE"
"$BIN" -license-status -license-file "$LICENSE"
sh ./deploy/install-systemd.sh
sudo systemctl restart chpp-middleware
```

การติดตั้ง service จะตรวจว่ามี license ที่ใช้งานได้ใน package หรือที่ `/opt/chpp-middleware/license.json` ก่อนเริ่ม service

Cleanup ทำงานในตัว service เอง ค่า default คือเก็บ log/queue ที่จบแล้ว 30 วัน:

```text
-cleanup-retention-days 30
-cleanup-interval-hours 24
```

ระบบจะไม่ลบ queue สถานะ PENDING/RETRYING เพื่อกันข้อมูลที่ยังไม่ได้ส่งหาย
