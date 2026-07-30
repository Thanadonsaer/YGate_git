# CHPP Modbus API Middleware

Go middleware สำหรับอ่าน Modbus TCP ตามตาราง configuration ใน SQLite แล้วส่ง KPI ไปยัง Y-Gate Solar API

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

Build ตอนนี้สร้าง Windows exe ไฟล์เดียว และ Linux Debian amd64 package

```bat
build-all.bat
```

ผลลัพธ์หลัก:

```text
build\middleware-v0.2.6-windows-amd64.exe
build\linux\build\middleware-linux-amd64
build\linux\deploy\*.sh
build\patches\chpp-middleware-v0.2.6-windows-amd64-update.zip
build\patches\chpp-middleware-v0.2.6-linux-amd64-update.zip
```


`build-all.bat` จะ clean output เก่าแบบ best-effort, รัน `go test ./...`, `go vet ./...` แล้ว build Windows amd64 exe + Linux Debian amd64 ถ้ามี process เก่าล็อกไฟล์ใน `build` ให้ปิดโปรแกรม/service เก่าก่อนแล้วรัน build อีกครั้ง

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

## Background service on Linux

ใช้ systemd เพื่อให้ middleware รันหลัง reboot และไม่ต้องเปิด SSH ค้างไว้ สามารถติดตั้งผ่าน menu ได้เลย:

```bash
cd ~/Downloads/pack
sh ./deploy/manage-service.sh
```

จากนั้นเลือก `1) Install / Update service` และเลือก `2) Info` เพื่อตรวจ version/path/db/status

ถ้าต้องการสั่ง installer ตรง ๆ ยังใช้ได้:

```bash
sh ./deploy/install-systemd.sh
```

ดูสถานะ/Log แบบ command ตรง:

```bash
systemctl status chpp-middleware --no-pager
journalctl -u chpp-middleware -f
```

แก้ค่าที่เครื่องปลายทาง:

```bash
sudo nano /etc/chpp-middleware.env
sudo systemctl restart chpp-middleware
```

Cleanup ทำงานในตัว service เอง ค่า default คือเก็บ log/queue ที่จบแล้ว 30 วัน:

```text
-cleanup-retention-days 30
-cleanup-interval-hours 24
```

ระบบจะไม่ลบ queue สถานะ PENDING/RETRYING เพื่อกันข้อมูลที่ยังไม่ได้ส่งหาย


