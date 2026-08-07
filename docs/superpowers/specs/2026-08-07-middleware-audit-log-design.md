# Middleware Audit Log

## Goal

ทำให้การดึง telemetry จาก Middleware ตรวจสอบได้จากเมนู Audit Log โดยเห็นผลลัพธ์และจุดที่ล้มเหลวแบบแยกขั้นตอน

## Design

ใช้ `audit_log` เดิม ไม่เพิ่มตารางใหม่ การ puller จะบันทึก event แบบ best-effort โดยใช้ actor เป็น system และ target เป็น middleware client เดิม

Actions:

- `middleware.pull.started`
- `middleware.pull.empty`
- `middleware.pull.succeeded`
- `middleware.pull.failed`
- `middleware.pull.ack_failed`

รายละเอียดใน `after_data` จะมี gateway, batch size, จำนวน rows, accepted/duplicate/rejected, ระยะเวลา และ error ตามกรณี

หน้า Audit Log จะมีตัวกรอง `ทั้งหมด` และ `Middleware Log` โดยกรองจาก action ที่ขึ้นต้นด้วย `middleware.pull.`

การเขียน audit จะไม่ทำให้ pull หยุด หากเขียนไม่สำเร็จให้เขียนข้อความลง service log และทำ flow เดิมต่อ

## Testing

เพิ่ม unit tests สำหรับการสร้าง middleware pull audit event และการแสดง/กรอง event ฝั่ง frontend จะตรวจด้วย typecheck/build ที่มีอยู่
