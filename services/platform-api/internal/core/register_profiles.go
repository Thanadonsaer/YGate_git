package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"ygate/platform-api/internal/auth"
	"ygate/platform-api/internal/database/dbgen"
)

type RegisterProfile struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	Name           string    `json:"name"`
	Manufacturer   string    `json:"manufacturer"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type RegisterProfileAddress struct {
	ID                 string                 `json:"id"`
	OrganizationID     string                 `json:"organizationId"`
	ProfileID          string                 `json:"profileId"`
	AddressKey         string                 `json:"addressKey"`
	DisplayName        string                 `json:"displayName"`
	Unit               string                 `json:"unit"`
	DataType           string                 `json:"dataType"`
	Scale              float64                `json:"scale"`
	Offset             float64                `json:"offset"`
	Decimals           int32                  `json:"decimals"`
	IsEnabled          bool                   `json:"isEnabled"`
	Notes              string                 `json:"notes"`
	ModbusFunctionCode *int32                 `json:"modbusFunctionCode"`
	ModbusRegister     *int32                 `json:"modbusRegister"`
	ModbusWordOrder    *string                `json:"modbusWordOrder"`
	ModbusDataType     *string                `json:"modbusDataType"`
	IsAlarm            bool                   `json:"isAlarm"`
	MappingMode        string                 `json:"mappingMode"`
	BitInterpretation  string                 `json:"bitInterpretation"`
	Mappings           []RegisterValueMapping `json:"mappings"`
	CreatedAt          time.Time              `json:"createdAt"`
	UpdatedAt          time.Time              `json:"updatedAt"`
}

type CreateRegisterProfileInput struct {
	OrganizationID string
	Name           string
	Manufacturer   string
	Description    string
}

type RegisterProfileAddressInput struct {
	AddressKey         string
	DisplayName        string
	Unit               string
	DataType           string
	Scale              float64
	Offset             float64
	Decimals           int32
	IsEnabled          bool
	Notes              string
	ModbusFunctionCode *int32
	ModbusRegister     *int32
	ModbusWordOrder    string
	ModbusDataType     string
	IsAlarm            bool
	MappingMode        string
	BitInterpretation  string
	Mappings           []RegisterValueMapping
}

func normalizeRegisterProfileName(value string) string { return strings.TrimSpace(value) }

func validateRegisterProfileAddress(input *RegisterProfileAddressInput) error {
	input.AddressKey = strings.TrimSpace(input.AddressKey)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Unit = strings.TrimSpace(input.Unit)
	input.DataType = strings.TrimSpace(input.DataType)
	input.Notes = strings.TrimSpace(input.Notes)
	input.ModbusWordOrder = strings.TrimSpace(input.ModbusWordOrder)
	input.ModbusDataType = strings.TrimSpace(input.ModbusDataType)
	input.MappingMode = strings.ToUpper(strings.TrimSpace(input.MappingMode))
	if input.DisplayName == "" {
		input.DisplayName = input.AddressKey
	}
	if input.DataType == "" {
		input.DataType = "number"
	}
	if input.Scale == 0 {
		input.Scale = 1
	}
	if input.MappingMode == "" {
		input.MappingMode = "EXACT"
	}
	if input.BitInterpretation == "" {
		input.BitInterpretation = "INDEPENDENT_FLAGS"
	}
	if input.AddressKey == "" || len(input.AddressKey) > 200 || len(input.DisplayName) > 200 || len(input.Unit) > 40 || len(input.Notes) > 500 || input.Decimals < 0 || input.Decimals > 9 || math.IsNaN(input.Scale) || math.IsInf(input.Scale, 0) || math.IsNaN(input.Offset) || math.IsInf(input.Offset, 0) {
		return ErrInvalid
	}
	if input.MappingMode != "EXACT" && input.MappingMode != "BITMASK" || input.MappingMode == "BITMASK" && input.BitInterpretation != "ONE_HOT" && input.BitInterpretation != "INDEPENDENT_FLAGS" {
		return ErrInvalid
	}
	if input.DataType != "number" && input.DataType != "boolean" && input.DataType != "text" && input.DataType != "enum" {
		return ErrInvalid
	}
	if err := validateModbusRegisterFields(input.ModbusFunctionCode, input.ModbusRegister, input.ModbusWordOrder, input.ModbusDataType); err != nil {
		return err
	}
	seenExact := map[int64]bool{}
	seenBit := map[int32]bool{}
	for _, mapping := range input.Mappings {
		if mapping.DisplayValue == "" || mapping.AlarmState != "" && !input.IsAlarm || mapping.Severity != "" && !input.IsAlarm || (mapping.MatchValue == nil) == (mapping.BitIndex == nil) {
			return ErrInvalid
		}
		if mapping.MatchValue != nil {
			if seenExact[*mapping.MatchValue] {
				return ErrInvalid
			}
			seenExact[*mapping.MatchValue] = true
		}
		if mapping.BitIndex != nil {
			if *mapping.BitIndex < 0 || *mapping.BitIndex > 63 || seenBit[*mapping.BitIndex] {
				return ErrInvalid
			}
			seenBit[*mapping.BitIndex] = true
		}
	}
	return nil
}

func (s *Service) ListRegisterProfiles(ctx context.Context, principal auth.Principal) ([]RegisterProfile, error) {
	if !principal.OrganizationID.Valid {
		return nil, ErrForbidden
	}
	if err := s.requireOrganizationPermission(ctx, s.queries, principal, "read", "device_model", principal.OrganizationID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, organization_id, name, manufacturer, description, created_at, updated_at FROM plant.register_profile WHERE organization_id=$1 ORDER BY name, id`, principal.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("list register profiles: %w", err)
	}
	defer rows.Close()
	profiles := []RegisterProfile{}
	for rows.Next() {
		profile, scanErr := scanRegisterProfile(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (s *Service) CreateRegisterProfile(ctx context.Context, principal auth.Principal, input CreateRegisterProfileInput, sourceIP *netip.Addr) (RegisterProfile, error) {
	organizationID := principal.OrganizationID
	if strings.TrimSpace(input.OrganizationID) != "" {
		var err error
		organizationID, err = parseUUID(input.OrganizationID)
		if err != nil {
			return RegisterProfile{}, ErrInvalid
		}
	}
	input.Name = normalizeRegisterProfileName(input.Name)
	input.Manufacturer = strings.TrimSpace(input.Manufacturer)
	input.Description = strings.TrimSpace(input.Description)
	if !organizationID.Valid || input.Name == "" || len(input.Name) > 200 || len(input.Description) > 500 {
		return RegisterProfile{}, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RegisterProfile{}, fmt.Errorf("begin register profile: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if err = s.requireOrganizationPermission(ctx, q, principal, "create", "device_model", organizationID); err != nil {
		return RegisterProfile{}, err
	}
	id, err := newUUID()
	if err != nil {
		return RegisterProfile{}, err
	}
	profile, err := scanRegisterProfile(tx.QueryRow(ctx, `INSERT INTO plant.register_profile(id, organization_id, name, manufacturer, description) VALUES($1,$2,$3,$4,$5) RETURNING id, organization_id, name, manufacturer, description, created_at, updated_at`, id, organizationID, input.Name, input.Manufacturer, input.Description))
	if err != nil {
		if strings.Contains(err.Error(), "register_profile_name_unique") {
			return RegisterProfile{}, ErrConflict
		}
		return RegisterProfile{}, fmt.Errorf("create register profile: %w", err)
	}
	correlationID, err := newUUID()
	if err != nil {
		return RegisterProfile{}, err
	}
	after, _ := json.Marshal(profile)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "register_profile.created", TargetType: "register_profile", TargetID: id, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return RegisterProfile{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RegisterProfile{}, fmt.Errorf("commit register profile: %w", err)
	}
	return profile, nil
}

func (s *Service) ListRegisterProfileAddresses(ctx context.Context, principal auth.Principal, profileID string) ([]RegisterProfileAddress, error) {
	id, err := parseUUID(profileID)
	if err != nil {
		return nil, ErrNotFound
	}
	var organizationID pgtype.UUID
	if err = s.pool.QueryRow(ctx, `SELECT organization_id FROM plant.register_profile WHERE id=$1`, id).Scan(&organizationID); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if err = s.requireOrganizationPermission(ctx, s.queries, principal, "read", "device_model", organizationID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, organization_id, profile_id, address_key, display_name, unit, data_type, scale, value_offset, decimals, is_enabled, notes, modbus_function_code, modbus_register, modbus_word_order, modbus_data_type, is_alarm, mapping_mode, bit_interpretation, created_at, updated_at FROM plant.register_profile_address WHERE organization_id=$1 AND profile_id=$2 ORDER BY address_key`, organizationID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	addresses := []RegisterProfileAddress{}
	for rows.Next() {
		address, scanErr := scanRegisterProfileAddress(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		mappingRows, queryErr := s.pool.Query(ctx, `SELECT match_value, bit_index, display_value, COALESCE(alarm_state,''), COALESCE(severity,''), sort_order FROM plant.register_value_mapping WHERE organization_id=$1 AND profile_address_id=$2 ORDER BY sort_order, bit_index NULLS FIRST, match_value NULLS FIRST`, organizationID, parseUUIDMust(address.ID))
		if queryErr != nil {
			return nil, queryErr
		}
		for mappingRows.Next() {
			var mapping RegisterValueMapping
			var matchValue pgtype.Int8
			var bitIndex pgtype.Int4
			if scanErr = mappingRows.Scan(&matchValue, &bitIndex, &mapping.DisplayValue, &mapping.AlarmState, &mapping.Severity, new(int32)); scanErr != nil {
				mappingRows.Close()
				return nil, scanErr
			}
			if matchValue.Valid {
				value := matchValue.Int64
				mapping.MatchValue = &value
			}
			if bitIndex.Valid {
				value := bitIndex.Int32
				mapping.BitIndex = &value
			}
			address.Mappings = append(address.Mappings, mapping)
		}
		mappingRows.Close()
		addresses = append(addresses, address)
	}
	return addresses, rows.Err()
}

func (s *Service) AssignRegisterProfile(ctx context.Context, principal auth.Principal, modelID, profileID string, sourceIP *netip.Addr) error {
	model, err := parseUUID(modelID)
	if err != nil {
		return ErrNotFound
	}
	profile, err := parseUUID(profileID)
	if err != nil {
		return ErrNotFound
	}
	var organizationID pgtype.UUID
	if err = s.pool.QueryRow(ctx, `SELECT organization_id FROM plant.device_model WHERE id=$1`, model).Scan(&organizationID); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	if err = s.requireOrganizationPermission(ctx, q, principal, "update", "device_model", organizationID); err != nil {
		return err
	}
	var profileOrganization pgtype.UUID
	if err = tx.QueryRow(ctx, `SELECT organization_id FROM plant.register_profile WHERE id=$1`, profile).Scan(&profileOrganization); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil || !profileOrganization.Valid || profileOrganization != organizationID {
		return ErrInvalid
	}
	if _, err = tx.Exec(ctx, `UPDATE plant.device_model SET register_profile_id=$2, updated_at=now() WHERE organization_id=$1 AND id=$3`, organizationID, profile, model); err != nil {
		return err
	}
	correlationID, err := newUUID()
	if err != nil {
		return err
	}
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "device_model.register_profile_assigned", TargetType: "device_model", TargetID: model, AfterData: json.RawMessage(fmt.Sprintf(`{"registerProfileId":%q}`, uuidString(profile))), SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) UpsertRegisterProfileAddress(ctx context.Context, principal auth.Principal, profileID string, input RegisterProfileAddressInput, sourceIP *netip.Addr) (RegisterProfileAddress, error) {
	profile, err := parseUUID(profileID)
	if err != nil {
		return RegisterProfileAddress{}, ErrNotFound
	}
	if err = validateRegisterProfileAddress(&input); err != nil {
		return RegisterProfileAddress{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RegisterProfileAddress{}, err
	}
	defer tx.Rollback(ctx)
	q := s.queries.WithTx(tx)
	var organizationID pgtype.UUID
	if err = tx.QueryRow(ctx, `SELECT organization_id FROM plant.register_profile WHERE id=$1`, profile).Scan(&organizationID); errors.Is(err, pgx.ErrNoRows) {
		return RegisterProfileAddress{}, ErrNotFound
	} else if err != nil {
		return RegisterProfileAddress{}, err
	}
	if err = s.requireOrganizationPermission(ctx, q, principal, "update", "device_model", organizationID); err != nil {
		return RegisterProfileAddress{}, err
	}
	id, err := newUUID()
	if err != nil {
		return RegisterProfileAddress{}, err
	}
	address, err := scanRegisterProfileAddress(tx.QueryRow(ctx, `
INSERT INTO plant.register_profile_address (
 id, organization_id, profile_id, address_key, display_name, unit, data_type,
 scale, value_offset, decimals, is_enabled, notes, modbus_function_code,
 modbus_register, modbus_word_order, modbus_data_type, is_alarm, mapping_mode, bit_interpretation
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
ON CONFLICT (organization_id, profile_id, address_key) DO UPDATE SET
 display_name=EXCLUDED.display_name, unit=EXCLUDED.unit, data_type=EXCLUDED.data_type,
 scale=EXCLUDED.scale, value_offset=EXCLUDED.value_offset, decimals=EXCLUDED.decimals,
 is_enabled=EXCLUDED.is_enabled, notes=EXCLUDED.notes,
 modbus_function_code=EXCLUDED.modbus_function_code, modbus_register=EXCLUDED.modbus_register,
 modbus_word_order=EXCLUDED.modbus_word_order, modbus_data_type=EXCLUDED.modbus_data_type,
 is_alarm=EXCLUDED.is_alarm, mapping_mode=EXCLUDED.mapping_mode, bit_interpretation=EXCLUDED.bit_interpretation, updated_at=now()
RETURNING id, organization_id, profile_id, address_key, display_name, unit, data_type,
 scale, value_offset, decimals, is_enabled, notes, modbus_function_code, modbus_register,
 modbus_word_order, modbus_data_type, is_alarm, mapping_mode, bit_interpretation, created_at, updated_at`,
		id, organizationID, profile, input.AddressKey, input.DisplayName, input.Unit, input.DataType,
		input.Scale, input.Offset, input.Decimals, input.IsEnabled, input.Notes,
		nullableInt32(input.ModbusFunctionCode), nullableInt32(input.ModbusRegister), nullableText(input.ModbusWordOrder), nullableText(input.ModbusDataType), input.IsAlarm, input.MappingMode, input.BitInterpretation))
	if err != nil {
		return RegisterProfileAddress{}, mapWriteError(err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM plant.register_value_mapping WHERE organization_id=$1 AND profile_address_id=$2`, organizationID, parseUUIDMust(address.ID)); err != nil {
		return RegisterProfileAddress{}, err
	}
	for index, mapping := range input.Mappings {
		mappingID, idErr := newUUID()
		if idErr != nil {
			return RegisterProfileAddress{}, idErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO plant.register_value_mapping(id, organization_id, profile_address_id, match_value, bit_index, display_value, alarm_state, severity, sort_order) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9)`, mappingID, organizationID, parseUUIDMust(address.ID), nullableInt64(mapping.MatchValue), nullableInt32(mapping.BitIndex), mapping.DisplayValue, mapping.AlarmState, mapping.Severity, index); err != nil {
			return RegisterProfileAddress{}, mapWriteError(err)
		}
		address.Mappings = append(address.Mappings, mapping)
	}
	correlationID, err := newUUID()
	if err != nil {
		return RegisterProfileAddress{}, err
	}
	after, _ := json.Marshal(address)
	if err = q.CreateAuditEventFull(ctx, dbgen.CreateAuditEventFullParams{OrganizationID: organizationID, ActorUserID: principal.UserID, Action: "register_profile_address.updated", TargetType: "register_profile", TargetID: profile, AfterData: after, SourceIp: sourceIP, CorrelationID: correlationID}); err != nil {
		return RegisterProfileAddress{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RegisterProfileAddress{}, err
	}
	return address, nil
}

func nullableInt64(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func parseUUIDMust(value string) pgtype.UUID { id, _ := parseUUID(value); return id }

func scanRegisterProfile(row registerMetadataScanner) (RegisterProfile, error) {
	var profile RegisterProfile
	var id, organizationID pgtype.UUID
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &organizationID, &profile.Name, &profile.Manufacturer, &profile.Description, &createdAt, &updatedAt); err != nil {
		return RegisterProfile{}, err
	}
	profile.ID, profile.OrganizationID = uuidString(id), uuidString(organizationID)
	profile.CreatedAt, profile.UpdatedAt = createdAt.Time, updatedAt.Time
	return profile, nil
}

func scanRegisterProfileAddress(row registerMetadataScanner) (RegisterProfileAddress, error) {
	var address RegisterProfileAddress
	var id, organizationID, profileID pgtype.UUID
	var functionCode, register pgtype.Int4
	var wordOrder, dataType pgtype.Text
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &organizationID, &profileID, &address.AddressKey, &address.DisplayName, &address.Unit, &address.DataType, &address.Scale, &address.Offset, &address.Decimals, &address.IsEnabled, &address.Notes, &functionCode, &register, &wordOrder, &dataType, &address.IsAlarm, &address.MappingMode, &address.BitInterpretation, &createdAt, &updatedAt); err != nil {
		return RegisterProfileAddress{}, err
	}
	address.ID, address.OrganizationID, address.ProfileID = uuidString(id), uuidString(organizationID), uuidString(profileID)
	address.ModbusFunctionCode, address.ModbusRegister, address.ModbusWordOrder, address.ModbusDataType = int32Pointer(functionCode), int32Pointer(register), textPointer(wordOrder), textPointer(dataType)
	address.CreatedAt, address.UpdatedAt = createdAt.Time, updatedAt.Time
	address.Mappings = []RegisterValueMapping{}
	return address, nil
}
