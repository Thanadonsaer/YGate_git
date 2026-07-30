# Enterprise UX/UI Principles for Solar Plant SCADA

## 1. Mission

ออกแบบระบบ Solar Plant SCADA / Monitoring ที่ผู้ใช้เข้าใจสถานะของโรงไฟฟ้า ค้นหาปัญหา วิเคราะห์ข้อมูล และทำงานประจำได้อย่างรวดเร็ว ปลอดภัย และมั่นใจ

Brand identity ต้องสนับสนุนประสบการณ์ ไม่ใช่ควบคุมประสบการณ์

---

## 2. Priority Order

เมื่อเกิดความขัดแย้ง ให้เรียงลำดับความสำคัญดังนี้:

1. Operational safety and security
2. Accuracy and data integrity
3. Alarm and abnormal-state visibility
4. Task completion and workflow efficiency
5. Accessibility and readability
6. Information hierarchy
7. Responsiveness and performance
8. Visual consistency
9. Brand decoration

ห้ามลด usability เพื่อให้หน้าจอดูตรง CI มากขึ้น

---

## 3. Design Freedom

ทีมออกแบบและ AI สามารถออกแบบใหม่ได้จาก first principles ในหัวข้อต่อไปนี้:

- Application shell
- Navigation
- Sidebar
- Header
- Search
- Page hierarchy
- Dashboard composition
- Widget design
- Tables
- Forms
- Filters
- Toolbars
- Dialogs and drawers
- Charts
- Inspector panels
- Empty states
- Loading states
- Error recovery
- Responsive behavior
- Keyboard interaction
- Motion and transitions

ไม่ต้องรักษา Layout ของเว็บไซต์ Yokogawa หรือ dashboard รุ่นเดิม หากแนวทางใหม่ทำให้ใช้งานดีขึ้น

---

## 4. User-Centered Workflow

ทุกหน้าต้องตอบคำถามให้ชัดเจน:

- ผู้ใช้เข้าหน้านี้เพื่อทำอะไร
- ข้อมูลสำคัญที่สุดคืออะไร
- Action หลักคืออะไร
- ผู้ใช้รู้ได้อย่างไรว่างานสำเร็จ
- เมื่อเกิดข้อผิดพลาด ผู้ใช้แก้ต่ออย่างไร

ลดขั้นตอนที่ไม่จำเป็น แต่ไม่ซ่อนข้อมูลสำคัญหรือทำให้ action อันตรายเกิดขึ้นโดยไม่ตั้งใจ

---

## 5. Information Architecture

จัดกลุ่มเมนูตาม mental model และงานของผู้ใช้ ไม่ใช่ตามโครงสร้างฐานข้อมูลหรือ package ในโค้ด

แนวทางหลัก:

- ใช้ชื่อเมนูที่ผู้ใช้เข้าใจ
- แยก Overview, Operations, Analysis, Configuration และ Administration ให้ชัด
- แสดง context ของ Organization, Plant และ Device ที่กำลังดู
- รักษา filter/context เมื่อผู้ใช้เปลี่ยนหน้าเมื่อเหมาะสม
- ใช้ breadcrumbs เฉพาะเมื่อ hierarchy มีประโยชน์จริง
- หลีกเลี่ยง navigation ซ้อนหลายระดับเกินจำเป็น

---

## 6. Dashboard Overview

Dashboard Overview เป็น responsive information workspace ไม่ใช่ fixed SCADA canvas

### Goals

- เห็นสถานะ Plant ภายในเวลาอันสั้น
- แยก Normal, Warning, Alarm, Offline และ Stale ได้ทันที
- เจาะจากภาพรวมไปยัง Plant, Device หรือ point ที่ผิดปกติได้
- เปรียบเทียบ performance และ trend โดยไม่ต้องเปิดหลายหน้า

### Design Guidance

- ใช้ visual hierarchy ตามความสำคัญ ไม่ใช่ทำทุก card เด่นเท่ากัน
- KPI card ควรมี context, unit, timestamp และ quality เมื่อจำเป็น
- แสดง active filters และ time range อย่างชัดเจน
- รองรับ dense mode สำหรับ control room และ comfortable mode สำหรับงานทั่วไปได้
- Widget สามารถ drag/resize ได้ แต่ default layout ต้องดีโดยไม่ต้องปรับเอง
- อย่าใช้ card เป็นคำตอบสำหรับทุกอย่าง
- อนุญาต split view, pinned panel, inline drill-down หรือ detail drawer เมื่อช่วยลด context switching

---

## 7. SCADA Builder and Viewer

SCADA Builder เป็น fixed-canvas design tool ส่วน Viewer เป็น operational screen

### Builder

- แยกโหมด Edit, Preview และ Published Viewer ชัดเจน
- แสดง selection, snap, alignment และ resize affordance ที่เข้าใจง่าย
- Properties panel ต้องจัดกลุ่มข้อมูลเป็น Content, Binding, Appearance และ Behavior
- รองรับ undo/redo, copy/paste และ keyboard shortcuts
- แจ้งสถานะ Saving, Saved และ Conflict
- ป้องกันการสูญเสียงาน

### Viewer

- ซ่อน editor controls
- ให้ alarm, stale และ data quality state มีรูปแบบกลางที่ custom style กลบไม่ได้
- รองรับ fit-to-screen, fullscreen และ zoom อย่างเหมาะสม
- ห้ามใช้ animation ตกแต่งที่รบกวนการเฝ้าระวัง
- Dynamic animation ใช้ได้เมื่อสื่อ operational state และไม่ทำให้เข้าใจผิด

---

## 8. Tables and Dense Data

ตารางเป็นองค์ประกอบหลักของระบบ Enterprise และต้องได้รับการออกแบบอย่างจริงจัง

- ใช้ column alignment ตามชนิดข้อมูล
- ตัวเลขจัดแนวให้อ่านเปรียบเทียบง่าย
- แสดงหน่วยที่ header หรือ cell อย่างสม่ำเสมอ
- รองรับ sort, filter, search, column visibility และ saved view ตามความจำเป็น
- ใช้ sticky header หรือ pinned columns เมื่อช่วยงานจริง
- แสดง loading, empty, partial, stale และ error state
- การเลือกหลายรายการต้องชัดเจนและไม่ทำ action โดยไม่ตั้งใจ
- อย่าซ่อนข้อมูลสำคัญไว้หลัง tooltip เพียงอย่างเดียว

---

## 9. Forms and Configuration

- แบ่ง form ยาวเป็น section ตามงานของผู้ใช้
- ใช้ progressive disclosure สำหรับ advanced settings
- Validation ควรเกิดใกล้ field และอธิบายวิธีแก้
- แยก required, optional และ inherited value ให้ชัด
- แสดง unsaved changes
- destructive action ต้องสื่อผลกระทบอย่างชัดเจน
- ใช้ sensible defaults แต่ห้ามสร้างค่า configuration สำคัญแบบเงียบ
- รองรับ keyboard และ screen reader

---

## 10. Alarm and Status Design

- ไม่ใช้สีเพียงอย่างเดียว
- ใช้ icon, label, shape หรือ pattern ร่วมกับสี
- กำหนด semantic tokens กลางสำหรับ severity, quality และ communication state
- Critical state ต้องเด่นกว่าสีแบรนด์
- แสดง timestamp, source และ acknowledgment state เมื่อเกี่ยวข้อง
- ลด alarm fatigue ด้วย grouping, filtering และ clear prioritization
- ห้ามใช้ animation กระพริบต่อเนื่องโดยไม่จำเป็น

---

## 11. Charts and Data Visualization

เลือก chart จากคำถามที่ผู้ใช้ต้องตอบ ไม่ใช่จากความสวยงาม

- Trend over time: line/area ตามความเหมาะสม
- Comparison: bar หรือ aligned values
- Composition: stacked chart เมื่ออ่านได้จริง
- Distribution: histogram/box plot เมื่อผู้ใช้ต้องวิเคราะห์การกระจาย
- Relationship: scatter plot เมื่อมีความหมายทางวิศวกรรม

ทุก chart ควรพิจารณา:

- Unit
- Timezone
- Time range
- Aggregation
- Data quality
- Missing data
- Legend
- Tooltip
- Zoom or brush เมื่อข้อมูลยาว
- Accessible color distinction

หลีกเลี่ยง 3D chart, decorative gauge จำนวนมาก และ visual effect ที่บิดเบือนข้อมูล

---

## 12. Responsive Design

Responsive ไม่ได้หมายถึงย่อ desktop ลงมือถืออย่างเดียว

- ปรับ hierarchy และ action priority ตามพื้นที่
- Table อาจเปลี่ยนเป็น focused columns, details view หรือ horizontal scroll อย่างมีเหตุผล
- Control-room view สามารถเน้นจอใหญ่และ information density สูง
- Admin/configuration pages ต้องยังใช้งานได้บน laptop และ tablet ตาม requirement
- ทดสอบ breakpoint ด้วยข้อมูลจริง ไม่ใช่ placeholder สั้น ๆ

---

## 13. Accessibility

เป้าหมายขั้นต่ำ:

- Keyboard navigation
- Visible focus
- Semantic HTML
- Accessible name สำหรับ controls
- Contrast ที่เหมาะสม
- ไม่พึ่งสีเพียงอย่างเดียว
- รองรับ zoom และ text scaling
- Label และ error message ที่ screen reader เข้าใจได้
- Motion reduction ตาม user preference

Accessibility เป็น requirement ไม่ใช่งานตกแต่งรอบสุดท้าย

---

## 14. Motion

Motion ใช้เพื่อ:

- อธิบายความสัมพันธ์ของ state
- ให้ feedback
- ช่วย orientation
- แสดง transition ที่มีความหมาย

หลีกเลี่ยง:

- Decorative animation บน operational page
- Animation ที่ทำให้ chart หรือค่า telemetry ดูเหมือนเปลี่ยนทั้งที่ข้อมูลไม่เปลี่ยน
- Long transitions ที่ทำให้งานช้า
- Infinite motion ที่รบกวนสมาธิ

---

## 15. Loading, Empty, Error and Stale States

ทุก data-driven component ต้องออกแบบอย่างน้อย:

- Initial loading
- Refreshing
- Empty
- No result from filter
- Partial data
- Stale data
- Bad quality
- Permission denied
- Network error
- Server error

ข้อความต้องบอกสิ่งที่เกิดขึ้น ผลกระทบ และ action ต่อไปเมื่อมี

---

## 16. Design System Principles

- ใช้ semantic tokens แทน hard-coded visual values
- แยก brand tokens, functional tokens และ component tokens
- Component API ต้องรองรับ accessibility และ operational states
- ไม่สร้าง variant จำนวนมากโดยไม่มี use case
- Pattern ใหม่ต้องแก้ปัญหาจริงและใช้ซ้ำได้
- Brand Yellow เป็น accent ไม่ใช่ default color ของทุก primary action
- ไม่เดาค่า CI ที่ยังไม่ได้รับอนุมัติ

---

## 17. Anti-Patterns

ห้ามทำสิ่งต่อไปนี้โดยไม่มีเหตุผลที่ชัดเจน:

- เลียนแบบหน้าเว็บองค์กรสำหรับ application UI
- ใช้ card ซ้อน card หลายชั้น
- Sidebar หลายระดับที่เปิดพร้อมกันทั้งหมด
- ทุก action เป็นปุ่มสีเหลือง
- ทุกข้อมูลถูกซ่อนใน modal
- ใช้ icon โดยไม่มี label ใน action ที่ไม่เป็นสากล
- Dashboard ที่ทุก widget มีขนาดและน้ำหนักเท่ากัน
- Fixed height ที่ตัดข้อมูลจริง
- Hover-only interaction สำหรับ action สำคัญ
- Skeleton loading ที่ทำให้ layout กระโดด
- Animation เพื่อความล้ำสมัยโดยไม่มีความหมาย

---

## 18. Definition of Good UX

งานออกแบบถือว่าดีเมื่อ:

- ผู้ใช้ระบุสถานะ Plant และปัญหาหลักได้อย่างรวดเร็ว
- ผู้ใช้ทำงานหลักได้โดยไม่ต้องจำโครงสร้างระบบ
- Alarm และ data-quality issue ไม่ถูก brand decoration กลบ
- ผู้ใช้เข้าใจ context, time range, timezone, unit และ source ของข้อมูล
- หน้าจอรองรับข้อมูลจริงทั้งกรณีน้อย มาก ผิดพลาด และไม่ครบ
- Keyboard, responsive และ accessibility ใช้งานได้
- Visual design ยังจดจำ Yokogawa ได้โดยไม่ลดประสิทธิภาพงาน

---

## 17. UI Dependencies and Libraries

สามารถติดตั้ง dependency สำหรับงาน UX/UI ได้เมื่อช่วยลดโค้ดเฉพาะกิจ ลดความซ้ำซ้อน เพิ่มคุณภาพของ interaction หรือทำให้ implementation กระชับและตรวจสอบได้ง่ายขึ้น

เป้าหมายคือใช้ token และเวลาในการสร้างสิ่งที่มีคุณค่าต่อผลิตภัณฑ์ ไม่ใช่เขียน infrastructure หรือ primitive ที่มีไลบรารีมาตรฐานแก้ปัญหาได้ดีอยู่แล้ว

### Dependency Freedom

ทีมและ AI สามารถเพิ่ม UI dependency ได้โดยไม่ต้องหลีกเลี่ยงเพียงเพราะต้องการลดจำนวน package แต่ต้องเลือกอย่างมีเหตุผลและสอดคล้องกับ stack ของ repository

อนุญาตให้ใช้ dependency สำหรับงาน เช่น:

- Accessible UI primitives
- Form state and validation integration
- Data tables and virtualization
- Charts and visualization
- Date/time selection and formatting
- Drag and drop
- Command palette and search interaction
- Rich tooltips, popovers, dialogs and menus
- Keyboard shortcuts
- Responsive layout utilities
- Animation ที่มีความหมายต่อ interaction
- Testing and accessibility verification

### Selection Criteria

ให้เลือก dependency ที่:

1. เป็นที่นิยมและมีผู้ใช้งานจริงใน ecosystem เดียวกับโครงการ
2. มีการดูแลต่อเนื่อง มีเอกสารชัดเจน และมี release history ที่น่าเชื่อถือ
3. รองรับ TypeScript และ React/Next.js ได้ดี
4. รองรับ accessibility หรือเปิดทางให้ implement accessibility ได้ถูกต้อง
5. มี API ที่เสถียรและไม่บังคับ architecture ที่ขัดกับระบบ
6. ไม่เพิ่ม bundle size หรือ runtime cost เกินประโยชน์ที่ได้รับ
7. ไม่มี dependency chain ที่เสี่ยงหรือซ้ำซ้อนกับของเดิม
8. มี license ที่องค์กรใช้งานได้
9. สามารถทดสอบและอัปเกรดได้โดยไม่ผูกกับ vendor อย่างรุนแรง

### Preferred Approach

- ตรวจของเดิมใน repository ก่อนติดตั้งของใหม่
- ใช้ shared component และ design token เดิมเมื่อมีคุณภาพเพียงพอ
- เลือกไลบรารีเฉพาะทางที่ดีที่สุดสำหรับปัญหา แทนการสร้าง abstraction ขนาดใหญ่เอง
- ใช้ dependency หนึ่งตัวต่อหนึ่งความรับผิดชอบหลักเมื่อทำได้
- หลีกเลี่ยงการติดตั้งหลาย library ที่แก้ปัญหาเดียวกัน
- ห่อ third-party component ด้วย adapter บาง ๆ เมื่อจำเป็นต่อ consistency หรือการเปลี่ยน library ในอนาคต
- Import แบบเจาะจงและใช้ lazy loading/code splitting กับส่วนที่มีขนาดใหญ่
- อย่าคัดลอก source code จำนวนมากจาก library มาเก็บเองเพียงเพื่อหลีกเลี่ยง dependency

### Commonly Suitable Ecosystem Choices

ตัวอย่างกลุ่ม library ที่พิจารณาได้ โดยต้องตรวจ compatibility กับ repository ก่อนใช้จริง:

- Accessible primitives: Radix UI
- Headless data and server state: TanStack ecosystem
- Forms: React Hook Form ร่วมกับ Zod
- Drag and drop: dnd-kit
- Large lists/tables: TanStack Table และ TanStack Virtual
- Charts: Apache ECharts, Recharts หรือ library เดิมของ repositoryตามความซับซ้อนของงาน
- Date utilities: date-fns หรือ library เดิมของ repository
- Icons: icon set เดียวที่ tree-shakable และใช้สม่ำเสมอทั้งระบบ

รายการนี้เป็นตัวอย่าง ไม่ใช่คำสั่งให้ติดตั้งทั้งหมด

### Prohibited Dependency Behavior

ห้าม:

- ติดตั้ง package โดยไม่มี use case ที่ชัดเจน
- ใช้ library ใหม่เพื่อแทนของเดิมทั้งระบบโดยไม่จำเป็น
- เพิ่ม component framework ขนาดใหญ่หลายชุดพร้อมกัน
- ใช้ dependency ที่ abandoned, ไม่มี license ชัดเจน หรือมีประวัติความปลอดภัยที่ยังไม่แก้ไข
- ยอมให้ dependency กำหนด UX ที่แย่ลงเพียงเพราะใช้งานง่าย
- เพิ่ม infrastructure service ใหม่โดยอ้างว่าเป็น UI dependency

### Documentation Requirement

เมื่อเพิ่ม dependency ที่มีผลต่อ architecture หรือ bundle อย่างมีนัยสำคัญ ให้บันทึกสั้น ๆ ว่า:

- ใช้แก้ปัญหาอะไร
- เหตุใดของเดิมจึงไม่เพียงพอ
- ทางเลือกที่พิจารณา
- ผลต่อ bundle, accessibility และ maintenance
- วิธีทดสอบหรือถอดออกในอนาคต

ไม่จำเป็นต้องเขียน ADR สำหรับ utility package ขนาดเล็กทุกตัว เว้นแต่กฎ repository กำหนดไว้ แต่ต้องไม่ข้าม security review, license review หรือข้อกำหนดของ lockfile
