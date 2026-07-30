# Yokogawa Brand Guide

## 1. Purpose

เอกสารนี้กำหนดเฉพาะอัตลักษณ์ของแบรนด์ Yokogawa สำหรับงานผลิตภัณฑ์ดิจิทัล

เอกสารนี้ **ไม่ใช่ UX specification, design system หรือ layout specification** และห้ามใช้เพื่อจำกัด Navigation, Information Architecture, Component Design, Interaction Pattern, Dashboard Layout หรือ Data Visualization

---

## 2. Brand Essence

### Brand Slogan

**Co-innovating tomorrow**

### Brand Concept

**Connecting humanity and technology**

### Brand Personality

งานออกแบบควรสะท้อนบุคลิกต่อไปนี้ในระดับภาพรวม:

- Precise
- Innovative
- Trustworthy
- Human-centered
- Sustainable
- Confident without being aggressive
- Technical without becoming cold or difficult to use

Brand personality เป็นทิศทางการสื่อสาร ไม่ใช่ข้อบังคับของรูปทรง Layout หรือ Interaction

---

## 3. Brand Colors

### Primary Brand Accent

**Yokogawa Yellow**

ความหมาย:

- Energy
- Innovation
- Optimism
- Future
- Guidance

การใช้งานที่เหมาะสม:

- Brand highlight
- Selected or active state เมื่อ contrast ผ่านมาตรฐาน
- Important callout
- Key visualization highlight
- Marketing or onboarding moment

ข้อควรระวัง:

- ไม่ใช้สีเหลืองกับทุกปุ่ม ทุกไอคอน หรือทุกกราฟ
- ไม่ใช้สีเหลืองแทน warning โดยอัตโนมัติ
- ไม่ใช้สีแบรนด์กลบสถานะ alarm, quality หรือ severity
- ต้องตรวจ contrast และ readability ทุกครั้ง

### Structural Brand Color

**Deep Navy / Dark Blue**

ความหมาย:

- Stability
- Technology
- Precision
- Trust

การใช้งานที่เหมาะสม:

- Primary text
- Navigation structure
- Header, sidebar หรือ shell ในกรณีที่เหมาะกับบริบท
- Brand-supporting surfaces

### Neutral Colors

อนุญาตให้ใช้:

- White
- Light gray
- Mid gray
- Dark neutral
- Near-black

เลือกใช้ตาม readability, hierarchy, density และ operating environment

Dark UI, Light UI และ Hybrid UI สามารถใช้ได้ทั้งหมด หากยังคงการจดจำแบรนด์และผ่าน accessibility

### Functional Colors

ระบบสามารถเพิ่มสีเชิงหน้าที่ได้โดยไม่ต้องยึดกับสีแบรนด์ เช่น:

- Success
- Information
- Warning
- Major alarm
- Critical alarm
- Offline
- Stale data
- Bad quality
- Disabled
- Neutral

Functional colors ต้องมี semantic token และต้องไม่สื่อความหมายด้วยสีเพียงอย่างเดียว

> ห้ามเดาค่า HEX, RGB, CMYK หรือ Pantone ที่เป็นทางการ หากยังไม่ได้รับไฟล์ CI ที่อนุมัติ ให้ใช้ semantic token และ placeholder ที่เปลี่ยนได้ภายหลัง

---

## 4. Logo

โลโก้ Yokogawa ประกอบด้วย:

- Brand symbol
- Wordmark `YOKOGAWA`

หลักการใช้งาน:

- ใช้ไฟล์โลโก้ที่ได้รับอนุมัติเท่านั้น
- รักษาอัตราส่วนและพื้นที่ว่างรอบโลโก้
- ห้ามวาดใหม่ บิด ยืด หมุน ใส่เงา หรือเปลี่ยนสีโดยพลการ
- ห้ามใช้โลโก้เป็น decoration ซ้ำจำนวนมาก
- ต้องเลือกเวอร์ชันโลโก้ให้ contrast กับพื้นหลัง

---

## 5. The Leading Square

The Leading Square เป็นองค์ประกอบกราฟิกเพื่อสนับสนุนแนวคิด co-innovation, guidance และ future direction

### Allowed Use

- Hero or welcome area
- Marketing communication
- Empty state หรือ onboarding ที่ต้องการ brand moment
- Cover, report title page หรือ presentation
- Section accent ที่ใช้เพียงเล็กน้อย

### Restrictions

- เป็นองค์ประกอบเสริม ไม่ใช่องค์ประกอบบังคับ
- ไม่ต้องปรากฏทุกหน้า ทุก card หรือทุก component
- ห้ามทำให้ข้อมูลสำคัญอ่านยาก
- ห้ามรบกวน workflow หรือเพิ่ม visual noise
- ห้ามใช้แทน status, severity, navigation หรือ affordance

---

## 6. Geometry and Visual Language

Sharp lines, geometric shapes และ soft curves สามารถใช้เป็น visual expression ได้ตามความเหมาะสม

องค์ประกอบเหล่านี้:

- เป็น optional decorative language
- ไม่บังคับให้ทุก component เป็นทรงเหลี่ยม
- ไม่บังคับ grid, spacing, border radius หรือ page composition
- ไม่ควรลด readability, scanability หรือ interaction clarity

เลือก visual treatment จากบริบทของหน้าจอและงานของผู้ใช้เป็นหลัก

---

## 7. Typography

หากมี corporate typeface ที่ได้รับอนุมัติ ให้ใช้ตาม license และข้อกำหนดองค์กร

หากยังไม่มีไฟล์หรือ specification ที่ยืนยันแล้ว:

- ใช้ system font หรือ product font ที่อ่านง่าย
- รองรับภาษาไทยและอังกฤษอย่างสมบูรณ์
- ให้ความสำคัญกับตัวเลข หน่วย เวลา และข้อมูลตาราง
- ห้ามเดาชื่อฟอนต์ทางการ
- ห้ามฝังหรือแจกจ่ายไฟล์ฟอนต์ที่ไม่มีสิทธิ์

Typography ต้องสนับสนุน usability มากกว่าการตกแต่ง

---

## 8. Brand Voice

ข้อความในผลิตภัณฑ์ควร:

- ชัดเจน
- กระชับ
- สุภาพ
- แม่นยำ
- ไม่คลุมเครือ
- อธิบายผลกระทบและวิธีแก้ไขเมื่อเกิดข้อผิดพลาด

หลีกเลี่ยงคำโฆษณาเกินจริงใน operational UI

---

## 9. Brand Freedom Clause

Corporate Identity กำหนดเฉพาะ:

- Brand essence
- Logo
- Approved brand colors
- Brand personality
- Optional graphic expressions

Corporate Identity **ไม่ได้กำหนด**:

- Navigation
- Information Architecture
- Page Layout
- Dashboard Layout
- Component Architecture
- Interaction Patterns
- Form Design
- Table Design
- Chart Design
- Motion Design
- Responsive Behavior
- Data Density
- Workflow

ทีมออกแบบและ AI มีอิสระในการออกแบบสิ่งเหล่านี้จากหลัก usability, accessibility, safety และ product requirements

---

## 10. Brand Do and Don't

### Do

- ใช้แบรนด์เพื่อสร้างความต่อเนื่องและความน่าเชื่อถือ
- ใช้สีเหลืองอย่างมีจุดประสงค์
- รักษาความชัดเจนของข้อมูลและสถานะระบบ
- เลือก Light, Dark หรือ Hybrid theme ตามบริบท
- ใช้ semantic design tokens เพื่อเปลี่ยนค่า CI ได้ง่าย

### Don't

- อย่าเลียนแบบ Layout ของเว็บไซต์ Yokogawa
- อย่าบังคับให้ทุกหน้าดูเหมือนสื่อการตลาด
- อย่ายัด Leading Square ลงในทุก component
- อย่าใช้สีแบรนด์แทน semantic status ทั้งหมด
- อย่าให้ decoration ลดพื้นที่ข้อมูลหรือเพิ่มจำนวนคลิก
- อย่าใช้คำว่า clean, minimal หรือ corporate เป็นเหตุผลในการลดความสามารถของ UX
