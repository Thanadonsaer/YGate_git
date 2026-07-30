package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"

	"chpp/modbus-api-middleware/internal/domain"
	"chpp/modbus-api-middleware/internal/profile"
)

func (s *Server) FullHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/config", s.configPage)
	mux.HandleFunc("/logs", s.logsPage)
	mux.HandleFunc("/service", http.NotFound)
	mux.HandleFunc("/api/service/", http.NotFound)
	mux.HandleFunc("/brands/", s.brandPage)
	mux.HandleFunc("/import-template", s.importPage)
	mux.HandleFunc("/template/device-set-address.csv", s.templateCSV)
	mux.HandleFunc("/template/connections.csv", s.connectionTemplateCSV)
	mux.HandleFunc("/template/devices.csv", s.deviceTemplateCSV)
	mux.HandleFunc("/api/gateway-config", s.gatewayConfig)
	mux.HandleFunc("/api/delivery-logs", s.deliveryLogs)
	mux.HandleFunc("/api/poll-logs", s.pollLogs)
	mux.HandleFunc("/api/import-device-set-address", s.importDeviceSetAddress)
	mux.HandleFunc("/api/import-connections", s.importConnections)
	mux.HandleFunc("/api/modbus-discovery", s.modbusDiscovery)
	mux.HandleFunc("/api/settings/export", s.exportSettings)
	mux.HandleFunc("/api/settings/import", s.importSettings)
	mux.HandleFunc("/api/update/status", s.updateStatus)
	mux.HandleFunc("/api/update/upload", s.updateUpload)
	mux.HandleFunc("/api/update/apply", s.updateApply)
	mux.HandleFunc("/api/update/rollback", s.updateRollback)
	mux.HandleFunc("/api/version", s.versionInfo)
	mux.HandleFunc("/api/brands", s.brands)
	mux.HandleFunc("/api/plants", s.plants)
	mux.HandleFunc("/api/device-sets", s.deviceSets)
	mux.HandleFunc("/api/device-profiles/sma", s.installSMAProfile)
	mux.HandleFunc("/api/connections", s.connections)
	mux.Handle("/", s.Handler())
	return s.licenseGate(mux)
}

func (s *Server) installSMAProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	brands, err := s.Store.Brands()
	if err != nil {
		writeError(w, 500, err)
		return
	}
	var brand domain.Brand
	for _, candidate := range brands {
		if strings.EqualFold(candidate.BrandName, "SMA") {
			brand = candidate
			break
		}
	}
	if brand.BrandID == 0 {
		brand, err = s.Store.SaveBrand(domain.Brand{BrandName: "SMA", BrandDescription: "SMA Sunny Central via SC-COM / Modbus TCP"})
		if err != nil {
			writeError(w, 400, err)
			return
		}
	}
	sets, err := s.Store.DeviceSets()
	if err != nil {
		writeError(w, 500, err)
		return
	}
	for _, set := range sets {
		if set.BrandID == brand.BrandID && strings.EqualFold(set.DevModel, profile.SMADeviceModel) {
			writeJSON(w, 200, set)
			return
		}
	}
	saved, err := s.Store.SaveDeviceSet(profile.SMADeviceSet(brand.BrandID))
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 201, saved)
}

func (s *Server) versionInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":        s.Version,
		"os":             runtime.GOOS,
		"arch":           runtime.GOARCH,
		"updateEnabled":  true,
		"canApplyUpdate": s.CanApplyUpdate,
	})
}

func (s *Server) plants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		v, err := s.Store.Plants()
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 200, v)
	case http.MethodPost:
		var v domain.Plant
		if !decode(w, r, &v) {
			return
		}
		saved, err := s.Store.SavePlant(v)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		writeJSON(w, 201, saved)
	case http.MethodDelete:
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err := s.Store.DeletePlant(id); err != nil {
			writeError(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"deleted": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data, "error": nil})
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
}

func (s *Server) gatewayConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		v, err := s.Store.GatewayConfig()
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 200, v)
	case http.MethodPost:
		var v domain.GatewayConfig
		if !decode(w, r, &v) {
			return
		}
		saved, err := s.Store.SaveGatewayConfig(v)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		writeJSON(w, 201, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) deliveryLogs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		logs, err := s.Store.DeliveryLogs(limit)
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 200, logs)
	case http.MethodDelete:
		deleted, err := s.Store.ClearDeliveryLogs()
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"deleted": deleted})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) pollLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	connectionID, _ := strconv.ParseInt(r.URL.Query().Get("connectionId"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	logs, err := s.Store.PollLogs(connectionID, limit)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, logs)
}

func (s *Server) brands(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		v, err := s.Store.Brands()
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 200, v)
	case http.MethodPost:
		var v domain.Brand
		if !decode(w, r, &v) {
			return
		}
		saved, err := s.Store.SaveBrand(v)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		writeJSON(w, 201, saved)
	case http.MethodDelete:
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err := s.Store.DeleteBrand(id); err != nil {
			writeError(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"deleted": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) deviceSets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		v, err := s.Store.DeviceSets()
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 200, v)
	case http.MethodPost:
		var v domain.DeviceSet
		if !decode(w, r, &v) {
			return
		}
		saved, err := s.Store.SaveDeviceSet(v)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		writeJSON(w, 201, saved)
	case http.MethodDelete:
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err := s.Store.DeleteDeviceSet(id); err != nil {
			writeError(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"deleted": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		v, err := s.Store.ConnectionsWithStatus()
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 200, v)
	case http.MethodPost:
		var v domain.ConnectionConfig
		if !decode(w, r, &v) {
			return
		}
		plant, err := s.Store.PlantByCode(v.PlantCode)
		if err != nil {
			writeError(w, 400, fmt.Errorf("create plant %q before creating device", v.PlantCode))
			return
		}
		v.PlantCode, v.PlantName = plant.PlantCode, plant.PlantName
		wantedEnabled := v.Enabled || v.ConnectionID == 0
		v.Enabled = wantedEnabled
		saved, err := s.Store.SaveConnection(v)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		if err = s.Store.SetConnectionEnabled(saved.ConnectionID, wantedEnabled); err != nil {
			writeError(w, 400, err)
			return
		}
		saved.Enabled = wantedEnabled
		writeJSON(w, 201, saved)
	case http.MethodPatch:
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		enabled, _ := strconv.ParseBool(r.URL.Query().Get("enabled"))
		if err := s.Store.SetConnectionEnabled(id, enabled); err != nil {
			writeError(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"connectionId": id, "enabled": enabled})
	case http.MethodDelete:
		id, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err := s.Store.DeleteConnection(id); err != nil {
			writeError(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"deleted": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
