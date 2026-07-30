package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"chpp/modbus-api-middleware/internal/domain"
)

type settingsBundle struct {
	App           string                    `json:"app"`
	Version       string                    `json:"version"`
	ExportedAt    time.Time                 `json:"exportedAt"`
	GatewayConfig domain.GatewayConfig      `json:"gatewayConfig"`
	Brands        []domain.Brand            `json:"brands"`
	DeviceSets    []domain.DeviceSet        `json:"deviceSets"`
	Plants        []domain.Plant            `json:"plants"`
	Connections   []domain.ConnectionConfig `json:"connections"`
}

type settingsImportSummary struct {
	Brands      int `json:"brands"`
	DeviceSets  int `json:"deviceSets"`
	Plants      int `json:"plants"`
	Connections int `json:"connections"`
}

func (s *Server) exportSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	bundle, err := s.settingsBundle()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="chpp-middleware-settings.json"`)
	_ = json.NewEncoder(w).Encode(bundle)
}

func (s *Server) importSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var bundle settingsBundle
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&bundle); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	summary, err := s.applySettingsBundle(bundle)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) settingsBundle() (settingsBundle, error) {
	cfg, err := s.Store.GatewayConfig()
	if err != nil {
		return settingsBundle{}, err
	}
	brands, err := s.Store.Brands()
	if err != nil {
		return settingsBundle{}, err
	}
	sets, err := s.Store.DeviceSets()
	if err != nil {
		return settingsBundle{}, err
	}
	plants, err := s.Store.Plants()
	if err != nil {
		return settingsBundle{}, err
	}
	connections, err := s.Store.Connections()
	if err != nil {
		return settingsBundle{}, err
	}
	return settingsBundle{App: updateAppName, Version: s.Version, ExportedAt: time.Now().UTC(), GatewayConfig: cfg, Brands: brands, DeviceSets: sets, Plants: plants, Connections: connections}, nil
}

func (s *Server) applySettingsBundle(bundle settingsBundle) (settingsImportSummary, error) {
	var summary settingsImportSummary
	if _, err := s.Store.SaveGatewayConfig(bundle.GatewayConfig); err != nil {
		return summary, err
	}
	brandID := map[int64]int64{}
	brandByName := map[string]domain.Brand{}
	for _, b := range bundle.Brands {
		oldID := b.BrandID
		b.BrandID = 0
		saved, err := s.Store.SaveBrand(b)
		if err != nil {
			return summary, err
		}
		brandID[oldID] = saved.BrandID
		brandByName[strings.ToLower(saved.BrandName)] = saved
		summary.Brands++
	}
	sets, err := s.Store.DeviceSets()
	if err != nil {
		return summary, err
	}
	setID := map[int64]int64{}
	for _, set := range bundle.DeviceSets {
		oldID := set.DeviceSetID
		set.DeviceSetID = 0
		if mapped := brandID[set.BrandID]; mapped != 0 {
			set.BrandID = mapped
		} else if brand := brandByName[strings.ToLower(set.BrandName)]; brand.BrandID != 0 {
			set.BrandID = brand.BrandID
		}
		for _, old := range sets {
			if old.BrandID == set.BrandID && strings.EqualFold(old.DevType, set.DevType) && strings.EqualFold(old.DevModel, set.DevModel) {
				set.DeviceSetID = old.DeviceSetID
				break
			}
		}
		saved, err := s.Store.SaveDeviceSet(set)
		if err != nil {
			return summary, err
		}
		setID[oldID] = saved.DeviceSetID
		summary.DeviceSets++
	}
	for _, plant := range bundle.Plants {
		plant.PlantID = 0
		if _, err = s.Store.SavePlant(plant); err != nil {
			return summary, err
		}
		summary.Plants++
	}
	existing, err := s.Store.Connections()
	if err != nil {
		return summary, err
	}
	byName, byDev := map[string]int64{}, map[string]int64{}
	for _, c := range existing {
		byName[strings.ToLower(c.ConnectionName)] = c.ConnectionID
		byDev[strings.ToLower(c.DevDn)] = c.ConnectionID
	}
	for _, c := range bundle.Connections {
		oldSetID := c.DeviceSetID
		if mapped := setID[oldSetID]; mapped != 0 {
			c.DeviceSetID = mapped
		}
		if c.DeviceSetID == 0 {
			return summary, fmt.Errorf("connection %q has unknown deviceSetId %d", c.ConnectionName, oldSetID)
		}
		if id := byDev[strings.ToLower(c.DevDn)]; id != 0 {
			c.ConnectionID = id
		} else if id = byName[strings.ToLower(c.ConnectionName)]; id != 0 {
			c.ConnectionID = id
		} else {
			c.ConnectionID = 0
		}
		wantedEnabled := c.Enabled
		saved, err := s.Store.SaveConnection(c)
		if err != nil {
			return summary, err
		}
		if err = s.Store.SetConnectionEnabled(saved.ConnectionID, wantedEnabled); err != nil {
			return summary, err
		}
		summary.Connections++
	}
	return summary, nil
}
