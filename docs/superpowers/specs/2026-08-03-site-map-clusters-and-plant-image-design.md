# Site Map Clusters and Plant Image Design

## Goal

เพิ่มการรวมหมุดแบบ cluster ในหน้า Site Map และรองรับรูปหลัก 1 รูปต่อ Plant เพื่อช่วยระบุ Site จากแผนที่และหน้าแก้ไข Plant

## Scope

### Site Map clustering

- ใช้ Leaflet marker-cluster integration ที่เข้ากับ react-leaflet
- หมุดแต่ละ Plant ยังคงใช้สีตามสถานะ communication
- เมื่อหลาย Plant อยู่ใกล้กันจะแสดง cluster count
- เมื่อซูมเข้า cluster จะแตกเป็นหมุดราย Plant
- แผนที่ยังใช้ fitBounds กับ Plant ที่มีพิกัด
- Plant ที่ไม่มีพิกัดไม่เข้าร่วม cluster และแสดงจำนวนที่ขาดพิกัดเหมือนเดิม

### Plant image

- แต่ละ Plant มีรูปหลักได้สูงสุด 1 รูป
- รองรับ PNG, JPEG, WebP ขนาดไม่เกิน 2 MiB
- รูปเก็บใน directory ที่กำหนดด้วย environment variable ของ platform API
- ฐานข้อมูลเก็บชื่อไฟล์/URL ที่ผูกกับ Plant
- เพิ่ม upload, preview และ remove ใน Plant editor
- Popup ของ Site Map แสดงรูปเมื่อมีรูป และ fallback เมื่อไม่มี
- การ upload/remove ใช้สิทธิ์เขียน Plant และตรวจ ownership/organization แบบเดียวกับการแก้ Plant
- ใช้ชื่อไฟล์ที่สร้างจาก UUID เท่านั้น ไม่ใช้ชื่อไฟล์จากผู้ใช้เพื่อป้องกัน path traversal
- เมื่อแทนที่หรือลบรูป ให้ลบไฟล์เก่าหลังจาก update สำเร็จ

## API and data flow

เพิ่ม nullable image_url ในตาราง plant และใน Plant response เป็น imageUrl.

เพิ่ม endpoints:

- POST /api/v1/plants/{plantId}/image รับ multipart field image
- DELETE /api/v1/plants/{plantId}/image
- GET /api/v1/plants/{plantId}/image/{filename} สำหรับส่งไฟล์รูป โดยตรวจ filename ที่เป็น UUID และตรวจ Plant ownership

Upload flow: browser ส่ง multipart ไป platform API → API จำกัดขนาด/ตรวจ MIME จาก bytes → เขียนไฟล์ UUID → update plant.image_url และ audit → ส่ง Plant ล่าสุดกลับมา.

## UI

Plant editor เพิ่ม image preview, file input, upload action และ remove action. การบันทึกข้อมูล Plant เดิมยังทำงานแยกจาก upload เพื่อไม่ให้การเปลี่ยนรูปทำให้ข้อมูลพิกัดหรือ metadata สูญหาย.

Popup ของ Site Map แสดงรูปในขนาดคงที่แบบ object-fit: cover และใช้ imageUrl ผ่าน gateway origin.

## Error handling

- ไฟล์เกินขนาด, MIME ไม่รองรับ หรืออ่านไฟล์ไม่ได้ → 400
- ไม่มีสิทธิ์หรือ Plant ไม่อยู่ในขอบเขต → 403/404 ตาม pattern เดิม
- เขียนไฟล์หรือฐานข้อมูลล้มเหลว → 500 และไม่เปลี่ยน URL ในฐานข้อมูลให้ชี้ไปยังไฟล์ที่ใช้ไม่ได้
- ถ้าโหลดรูปไม่ได้ Popup ยังแสดงข้อมูล Plant ต่อได้

## Verification

- backend unit tests สำหรับ image validation, UUID filename และ ownership
- frontend typecheck
- API contract consistency
- manual browser check: cluster แตกเมื่อ zoom, popup แสดงรูป, upload/replace/remove ทำงาน, และ Plant ไม่มีรูปยังแสดง fallback