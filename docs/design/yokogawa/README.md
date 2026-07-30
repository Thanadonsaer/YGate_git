# Yokogawa Product Design Guidelines

ชุดเอกสารนี้ใช้กำหนดแนวทาง Brand, UX/UI และคำสั่งสำหรับ AI/Codex โดยแยกความรับผิดชอบออกจากกันอย่างชัดเจน

## Files

1. `01_BRAND_GUIDE.md`
   - ใช้กำหนด Brand Identity เท่านั้น
   - ครอบคลุม Brand essence, สี, โลโก้, Leading Square และบุคลิกของแบรนด์
   - ไม่ใช้บังคับ Layout, Navigation, Component หรือ Interaction

2. `02_UI_UX_PRINCIPLES.md`
   - ใช้กำหนดหลัก UX/UI ของระบบ Enterprise SCADA/Monitoring
   - ให้ความสำคัญกับ usability, workflow efficiency, accessibility และ data clarity
   - เปิดอิสระให้ Codex ออกแบบ Navigation, Layout และ Components จากปัญหาของผู้ใช้จริง

3. `03_AI_DESIGN_PROMPT.md`
   - Prompt กลางสำหรับ Codex, GPT, Claude หรือ AI coding/design agents
   - ระบุลำดับความสำคัญและขอบเขตอิสระในการออกแบบ

## Recommended Location

```text
repo-root/
├── AGENTS.md
├── docs/
│   ├── architecture/
│   ├── requirements/
│   └── design/
│       └── yokogawa/
│           ├── README.md
│           ├── 01_BRAND_GUIDE.md
│           ├── 02_UI_UX_PRINCIPLES.md
│           └── 03_AI_DESIGN_PROMPT.md
```

## Instruction Precedence

เมื่อเอกสารขัดกัน ให้ใช้ลำดับดังนี้:

1. Product safety, security และ operational requirements
2. Acceptance criteria ของงานปัจจุบัน
3. `AGENTS.md`
4. `02_UI_UX_PRINCIPLES.md`
5. `01_BRAND_GUIDE.md`
6. Existing visual conventions

Brand guideline ห้าม override security, accessibility, operational state, alarm visibility หรือ usability

## Dependency Policy

สำหรับงาน UX/UI สามารถติดตั้ง dependency เพิ่มได้เมื่อช่วยลดโค้ดที่ไม่จำเป็น เพิ่ม accessibility หรือทำ interaction ที่ซับซ้อนได้อย่างน่าเชื่อถือ

ให้เลือก package ที่นิยม ใช้งานแพร่หลายใน ecosystem ของ React/Next.js, มีการดูแลต่อเนื่อง, รองรับ TypeScript, มี license ชัดเจน และไม่ซ้ำกับ dependency ที่มีอยู่แล้ว ห้ามติดตั้งทุก library ในรายการตัวอย่างโดยอัตโนมัติ ให้เลือกเฉพาะตัวที่แก้ปัญหาของงานปัจจุบันจริง

รายละเอียดอยู่ในหัวข้อ `UI Dependencies and Libraries` ของ `02_UI_UX_PRINCIPLES.md`

## How to Use with Codex

ให้เพิ่มข้อความนี้ใน task prompt:

```text
Read these files before designing or modifying UI:
- docs/design/yokogawa/01_BRAND_GUIDE.md
- docs/design/yokogawa/02_UI_UX_PRINCIPLES.md
- docs/design/yokogawa/03_AI_DESIGN_PROMPT.md

Treat the Brand Guide as branding guidance only.
Use the UX Principles as the source of truth for interaction, layout,
information architecture, accessibility and workflow design.
```
