package web

import (
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"chpp/modbus-api-middleware/internal/domain"
)

const importHTML = `<!doctype html>
<html lang="th"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Import Device Set Address</title>
<style>body{margin:0;background:#f4f7f8;color:#15242b;font:14px/1.45 "Segoe UI",Tahoma,sans-serif}.wrap{max-width:900px;margin:28px auto;padding:0 18px}.card{background:#fff;border:1px solid #dbe5e8;border-radius:12px;box-shadow:0 10px 28px rgba(17,47,56,.08);overflow:hidden}.head{padding:20px 22px;border-bottom:1px solid #dbe5e8;display:flex;justify-content:space-between;gap:12px}.head h1{margin:0;font-size:22px}.head p{margin:4px 0 0;color:#6b7c84}.body{padding:22px}.btn{border:0;border-radius:8px;min-height:40px;padding:0 16px;font-weight:800;cursor:pointer;text-decoration:none;display:inline-flex;align-items:center}.primary{background:linear-gradient(135deg,#18a66f,#117b58);color:#fff}.secondary{background:#edf3f4;color:#435f67}.drop{border:1px dashed #9bc6cf;border-radius:12px;padding:22px;background:#f8fcfc;margin:14px 0}.status{margin-top:14px;color:#6b7c84}.ok{color:#149466}.err{color:#bd4b52}.mono{font-family:Consolas,monospace;white-space:pre-wrap;background:#071e24;color:#a9d8c7;border-radius:8px;padding:12px;margin-top:12px;max-height:260px;overflow:auto}</style></head>
<body><main class="wrap"><section class="card"><div class="head"><div><h1>Import Device Set + Address</h1><p>ใช้ไฟล์ CSV จาก Export Template แล้ว import ครั้งเดียวแทนการเพิ่ม Address ทีละตัว</p></div><div><a class="btn secondary" href="/template/device-set-address.csv">Export Template</a> <a class="btn secondary" href="/">กลับ</a></div></div><div class="body"><div class="drop"><input id="file" type="file" accept=".csv,text/csv"><p>รองรับทั้ง CSV เดิม และ config เพิ่มเติม: address_mode, byte_order, word_order, max_block_size, address_length, address_word_order, offset, units, enabled</p><button class="btn primary" id="import" type="button">Import CSV</button></div><div class="status" id="status">เลือกไฟล์ CSV ที่แก้จาก Excel แล้วกด Import</div><div class="mono" id="raw" style="display:none"></div></div></section></main>
<script>
const f=document.getElementById('file'),s=document.getElementById('status'),raw=document.getElementById('raw');
function status(t,ok){s.textContent=t;s.className='status '+(ok?'ok':'err')}
document.getElementById('import').onclick=async()=>{if(!f.files.length)return status('กรุณาเลือกไฟล์ CSV ก่อน',false);const csv=await f.files[0].text();try{const r=await fetch('/api/import-device-set-address',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({csv})});const body=await r.json();if(!r.ok||body.success===false)throw new Error(body.error||'import failed');status('Import สำเร็จ: '+body.data.brands+' brand, '+body.data.deviceSets+' device set, '+body.data.addresses+' address',true);raw.style.display='block';raw.textContent=JSON.stringify(body.data,null,2)}catch(e){status(e.message,false)}};
</script></body></html>`

type csvImportRequest struct {
	CSV string `json:"csv"`
}

type connectionImportRequest struct {
	CSV       string `json:"csv"`
	PlantCode string `json:"plantCode"`
}

type importSummary struct {
	Brands     int `json:"brands"`
	DeviceSets int `json:"deviceSets"`
	Addresses  int `json:"addresses"`
}

type connectionImportSummary struct {
	Connections int `json:"connections"`
}

type importGroup struct {
	BrandName, DevType, DevModel      string
	AddressMode, ByteOrder, WordOrder string
	MaxBlockSize                      int
	Addresses                         []domain.Address
}

func (s *Server) importPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(importHTML))
}

func (s *Server) importDeviceSetAddress(w http.ResponseWriter, r *http.Request) {
	var req csvImportRequest
	if !decode(w, r, &req) {
		return
	}
	summary, err := s.importCSV(req.CSV)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	s.refreshCache()
	writeJSON(w, 201, summary)
}

func (s *Server) importConnections(w http.ResponseWriter, r *http.Request) {
	var req connectionImportRequest
	if !decode(w, r, &req) {
		return
	}
	summary, err := s.importConnectionCSV(req.CSV, req.PlantCode)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	s.refreshCache()
	writeJSON(w, 201, summary)
}

func (s *Server) importConnectionCSV(text string, selectedPlant ...string) (connectionImportSummary, error) {
	rows, err := connectionRows(text)
	if err != nil {
		return connectionImportSummary{}, err
	}
	if len(rows) < 2 {
		return connectionImportSummary{}, fmt.Errorf("CSV must include header and at least one row")
	}
	header := map[string]int{}
	for i, h := range rows[0] {
		header[csvHeaderKey(h)] = i
	}
	plantCode := ""
	if len(selectedPlant) > 0 {
		plantCode = strings.TrimSpace(selectedPlant[0])
	}
	need := []string{"connection_name", "device_code", "device_name", "host", "port", "unit_id", "enabled"}
	if _, ok := header["device_set_id"]; !ok {
		need = append(need, "brand_name", "dev_type", "dev_model")
	}
	if plantCode == "" {
		need = append(need, "plant_code")
	}
	for _, name := range need {
		if _, ok := header[name]; !ok {
			return connectionImportSummary{}, fmt.Errorf("missing column %s", name)
		}
	}
	plants, err := s.Store.Plants()
	if err != nil {
		return connectionImportSummary{}, err
	}
	plantByCode := map[string]domain.Plant{}
	for _, p := range plants {
		plantByCode[strings.ToLower(p.PlantCode)] = p
	}
	sets, err := s.Store.DeviceSets()
	if err != nil {
		return connectionImportSummary{}, err
	}
	setByID := map[int64]domain.DeviceSet{}
	for _, set := range sets {
		setByID[set.DeviceSetID] = set
	}
	existing, err := s.Store.ConnectionsWithStatus()
	if err != nil {
		return connectionImportSummary{}, err
	}
	byName, byDevice := map[string]int64{}, map[string]int64{}
	for _, c := range existing {
		byName[strings.ToLower(c.ConnectionName)] = c.ConnectionID
		byDevice[strings.ToLower(c.DevDn)] = c.ConnectionID
	}
	items := make([]domain.ConnectionConfig, 0, len(rows)-1)
	for line, row := range rows[1:] {
		if emptyCSVRow(row) {
			continue
		}
		name := csvValue(row, header, "connection_name")
		rowPlantCode := plantCode
		if rowPlantCode == "" {
			rowPlantCode = csvValue(row, header, "plant_code")
		}
		deviceCode := csvValue(row, header, "device_code")
		host := csvValue(row, header, "host")
		if name == "" || rowPlantCode == "" || deviceCode == "" || host == "" {
			return connectionImportSummary{}, fmt.Errorf("line %d: connection_name, plant_code, device_code and host are required", line+2)
		}
		plant, ok := plantByCode[strings.ToLower(rowPlantCode)]
		if !ok {
			return connectionImportSummary{}, fmt.Errorf("line %d: create plant %q first", line+2, rowPlantCode)
		}
		set, ok := resolveImportDeviceSet(row, header, sets, setByID)
		if !ok {
			return connectionImportSummary{}, fmt.Errorf("line %d: device set not found: device_set_id=%q brand=%q dev_type=%q dev_model=%q", line+2, csvValue(row, header, "device_set_id"), csvValue(row, header, "brand_name"), csvValue(row, header, "dev_type"), csvValue(row, header, "dev_model"))
		}
		port, e := strconv.Atoi(csvValue(row, header, "port"))
		if e != nil || port < 1 || port > 65535 {
			return connectionImportSummary{}, fmt.Errorf("line %d: invalid port", line+2)
		}
		unit, e := strconv.Atoi(csvValue(row, header, "unit_id"))
		if e != nil || unit < 0 || unit > 255 {
			return connectionImportSummary{}, fmt.Errorf("line %d: invalid unit_id", line+2)
		}
		enabled, e := strconv.ParseBool(csvValue(row, header, "enabled"))
		if e != nil {
			return connectionImportSummary{}, fmt.Errorf("line %d: enabled must be true or false", line+2)
		}
		id := byDevice[strings.ToLower(deviceCode)]
		if id == 0 {
			id = byName[strings.ToLower(name)]
		}
		items = append(items, domain.ConnectionConfig{ConnectionID: id, ConnectionName: name, DeviceSetID: set.DeviceSetID, PlantCode: plant.PlantCode, PlantName: plant.PlantName, DevDn: deviceCode, DeviceName: csvValue(row, header, "device_name"), Host: host, Port: port, UnitID: unit, Enabled: enabled})
	}
	if len(items) == 0 {
		return connectionImportSummary{}, fmt.Errorf("CSV has no connection rows")
	}
	for _, item := range items {
		wantedEnabled := item.Enabled
		saved, e := s.Store.SaveConnection(item)
		if e != nil {
			return connectionImportSummary{}, e
		}
		if e = s.Store.SetConnectionEnabled(saved.ConnectionID, wantedEnabled); e != nil {
			return connectionImportSummary{}, e
		}
	}
	return connectionImportSummary{Connections: len(items)}, nil
}

func connectionRows(text string) ([][]string, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	if strings.HasPrefix(strings.TrimSpace(text), "<") {
		var book struct {
			Rows []struct {
				Cells []struct {
					Index int    `xml:"Index,attr"`
					Data  string `xml:"Data"`
				} `xml:"Cell"`
			} `xml:"Worksheet>Table>Row"`
		}
		if err := xml.Unmarshal([]byte(text), &book); err != nil {
			return nil, fmt.Errorf("invalid Excel XML: %w", err)
		}
		rows := make([][]string, 0, len(book.Rows))
		for _, source := range book.Rows {
			row := []string{}
			for _, cell := range source.Cells {
				index := cell.Index
				if index < 1 {
					index = len(row) + 1
				}
				for len(row) < index {
					row = append(row, "")
				}
				row[index-1] = cell.Data
			}
			rows = append(rows, row)
		}
		return rows, nil
	}
	reader := csv.NewReader(strings.NewReader(text))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	return reader.ReadAll()
}

func (s *Server) importCSV(text string) (importSummary, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	reader := csv.NewReader(strings.NewReader(text))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil {
		return importSummary{}, err
	}
	if len(rows) < 2 {
		return importSummary{}, fmt.Errorf("CSV must include header and at least one row")
	}
	header := map[string]int{}
	for i, h := range rows[0] {
		header[csvHeaderKey(h)] = i
	}
	need := []string{"brand_name", "dev_type", "dev_model", "register", "description", "factor", "data_type"}
	for _, name := range need {
		if _, ok := header[name]; !ok {
			return importSummary{}, fmt.Errorf("missing column %s", name)
		}
	}
	groups := map[string]*importGroup{}
	for line, row := range rows[1:] {
		if emptyCSVRow(row) {
			continue
		}
		brand := csvValue(row, header, "brand_name")
		devType := csvValue(row, header, "dev_type")
		devModel := csvValue(row, header, "dev_model")
		if brand == "" || devType == "" || devModel == "" {
			return importSummary{}, fmt.Errorf("line %d: brand_name, dev_type and dev_model are required", line+2)
		}
		fc, err := csvFunctionCodeDefault(row, header, "address_fc", 3)
		if err != nil {
			return importSummary{}, fmt.Errorf("line %d: invalid address_fc", line+2)
		}
		register, err := strconv.Atoi(csvValue(row, header, "register"))
		if err != nil {
			return importSummary{}, fmt.Errorf("line %d: invalid register", line+2)
		}
		factor, err := strconv.ParseFloat(csvValue(row, header, "factor"), 64)
		if err != nil {
			return importSummary{}, fmt.Errorf("line %d: invalid factor", line+2)
		}
		key := strings.ToLower(brand) + "\x00" + strings.ToLower(devType) + "\x00" + strings.ToLower(devModel)
		g := groups[key]
		if g == nil {
			maxBlockSize, e := csvIntDefault(row, header, "max_block_size", 30)
			if e != nil || maxBlockSize < 1 || maxBlockSize > 125 {
				return importSummary{}, fmt.Errorf("line %d: invalid max_block_size", line+2)
			}
			g = &importGroup{BrandName: brand, DevType: devType, DevModel: devModel, AddressMode: csvDefault(row, header, "address_mode", "ZERO_BASED"), ByteOrder: csvDefault(row, header, "byte_order", "BIG_ENDIAN"), WordOrder: csvDefault(row, header, "word_order", "HIGH_LOW"), MaxBlockSize: maxBlockSize}
			groups[key] = g
		}
		offset, e := csvFloatDefault(row, header, "offset", 0)
		if e != nil {
			return importSummary{}, fmt.Errorf("line %d: invalid offset", line+2)
		}
		length, e := csvIntDefault(row, header, "address_length", 0)
		if e != nil || length < 0 || length > 4 {
			return importSummary{}, fmt.Errorf("line %d: invalid address_length", line+2)
		}
		enabled, e := csvEnabledDefault(row, header, true)
		if e != nil {
			return importSummary{}, fmt.Errorf("line %d: invalid enabled", line+2)
		}
		g.Addresses = append(g.Addresses, domain.Address{FunctionCode: fc, Register: register, Description: csvValue(row, header, "description"), CanonicalKey: csvValue(row, header, "canonical_key"), SourceTag: csvValue(row, header, "source_tag"), Factor: factor, Offset: offset, DataType: csvValue(row, header, "data_type"), Length: length, WordOrder: csvValue(row, header, "address_word_order"), SourceUnit: csvValue(row, header, "source_unit"), CanonicalUnit: csvValue(row, header, "canonical_unit"), Enabled: enabled, EnabledSet: true, Remark: csvValue(row, header, "remark")})
	}
	if len(groups) == 0 {
		return importSummary{}, fmt.Errorf("CSV has no address rows")
	}
	brands, err := s.Store.Brands()
	if err != nil {
		return importSummary{}, err
	}
	brandByName := map[string]domain.Brand{}
	for _, brand := range brands {
		brandByName[strings.ToLower(brand.BrandName)] = brand
	}
	sets, err := s.Store.DeviceSets()
	if err != nil {
		return importSummary{}, err
	}
	summary := importSummary{}
	seenBrands := map[int64]bool{}
	for _, g := range groups {
		brand, ok := brandByName[strings.ToLower(g.BrandName)]
		if !ok {
			brand, err = s.Store.SaveBrand(domain.Brand{BrandName: g.BrandName})
			if err != nil {
				return summary, err
			}
			brandByName[strings.ToLower(g.BrandName)] = brand
		}
		seenBrands[brand.BrandID] = true
		set := domain.DeviceSet{BrandID: brand.BrandID, DevType: g.DevType, DevModel: g.DevModel, AddressMode: g.AddressMode, ByteOrder: g.ByteOrder, WordOrder: g.WordOrder, MaxBlockSize: g.MaxBlockSize, Addresses: g.Addresses}
		for _, old := range sets {
			if old.BrandID == brand.BrandID && strings.EqualFold(old.DevType, g.DevType) && strings.EqualFold(old.DevModel, g.DevModel) {
				set.DeviceSetID = old.DeviceSetID
				break
			}
		}
		if _, err = s.Store.SaveDeviceSet(set); err != nil {
			return summary, err
		}
		summary.DeviceSets++
		summary.Addresses += len(g.Addresses)
	}
	summary.Brands = len(seenBrands)
	return summary, nil
}

func resolveImportDeviceSet(row []string, header map[string]int, sets []domain.DeviceSet, setByID map[int64]domain.DeviceSet) (domain.DeviceSet, bool) {
	if idText := csvValue(row, header, "device_set_id"); idText != "" {
		id, err := strconv.ParseInt(idText, 10, 64)
		if err == nil {
			set, ok := setByID[id]
			return set, ok
		}
		return domain.DeviceSet{}, false
	}
	brand, devType, model := csvValue(row, header, "brand_name"), csvValue(row, header, "dev_type"), csvValue(row, header, "dev_model")
	for _, set := range sets {
		if deviceSetKey(set.BrandName, set.DevType, set.DevModel) == deviceSetKey(brand, devType, model) {
			return set, true
		}
	}
	wantBrand, wantType, wantModel := looseDeviceSetText(brand), looseDeviceSetText(devType), looseDeviceSetText(model)
	var match domain.DeviceSet
	matched := 0
	for _, set := range sets {
		gotModel := looseDeviceSetText(set.DevModel)
		if looseDeviceSetText(set.BrandName) == wantBrand && looseDeviceSetText(set.DevType) == wantType && (gotModel == wantModel || strings.Contains(gotModel, wantModel) || strings.Contains(wantModel, gotModel)) {
			match = set
			matched++
		}
	}
	return match, matched == 1
}

func deviceSetKey(brand, devType, model string) string {
	return looseWords(brand) + "\x00" + looseWords(devType) + "\x00" + looseWords(model)
}

func looseWords(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func looseDeviceSetText(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
func csvValue(row []string, header map[string]int, name string) string {
	i, ok := header[csvHeaderKey(name)]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func csvHeaderKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "_", "-", "_").Replace(value)
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	return strings.Trim(value, "_")
}

func csvDefault(row []string, header map[string]int, name, fallback string) string {
	if value := csvValue(row, header, name); value != "" {
		return value
	}
	return fallback
}

func csvIntDefault(row []string, header map[string]int, name string, fallback int) (int, error) {
	value := csvValue(row, header, name)
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

func csvFunctionCodeDefault(row []string, header map[string]int, name string, fallback int) (int, error) {
	value := strings.TrimLeft(csvValue(row, header, name), "0")
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}
func csvFloatDefault(row []string, header map[string]int, name string, fallback float64) (float64, error) {
	value := csvValue(row, header, name)
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseFloat(value, 64)
}

func csvBoolDefault(row []string, header map[string]int, name string, fallback bool) (bool, error) {
	value := csvValue(row, header, name)
	if value == "" {
		return fallback, nil
	}
	return strconv.ParseBool(value)
}

func csvEnabledDefault(row []string, header map[string]int, fallback bool) (bool, error) {
	if _, ok := header["enabled"]; ok {
		return csvBoolDefault(row, header, "enabled", fallback)
	}
	state := strings.Trim(strings.ToLower(csvValue(row, header, "state")), " \"'")
	switch state {
	case "":
		return fallback, nil
	case "use", "used", "enable", "enabled", "true", "yes", "1":
		return true, nil
	case "not use", "not_use", "disable", "disabled", "false", "no", "0":
		return false, nil
	default:
		return false, fmt.Errorf("state must be use/not use or enabled must be true/false")
	}
}

func emptyCSVRow(row []string) bool {
	for _, v := range row {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}
