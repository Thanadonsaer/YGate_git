-- Resolve configured labels at read time. Numeric telemetry remains the
-- calculation/integration value; display_item_map is additive.
CREATE FUNCTION telemetry.display_data_items(
    p_organization_id uuid,
    p_device_model_id uuid,
    p_register_address_map jsonb
) RETURNS jsonb
LANGUAGE sql
STABLE
PARALLEL SAFE
AS $$
    SELECT COALESCE(jsonb_object_agg(item.key, labels.display_value) FILTER (WHERE labels.display_value IS NOT NULL), '{}'::jsonb)
    FROM jsonb_each_text(p_register_address_map) item
    LEFT JOIN plant.device_model dm ON dm.organization_id = p_organization_id AND dm.id = p_device_model_id
    LEFT JOIN plant.register_profile_address address
      ON address.organization_id = p_organization_id
     AND address.profile_id = dm.register_profile_id
     AND (address.address_key = item.key OR address.address_key = concat('reg', item.key))
    LEFT JOIN LATERAL (
        SELECT CASE
            WHEN address.mapping_mode = 'EXACT' THEN (
                SELECT NULLIF(btrim(mapping.display_value), '')
                FROM plant.register_value_mapping mapping
                WHERE mapping.organization_id = address.organization_id
                  AND mapping.profile_address_id = address.id
                  AND mapping.match_value = round(item.value::numeric)::bigint
                ORDER BY mapping.sort_order, mapping.id
                LIMIT 1
            )
            WHEN address.mapping_mode = 'BITMASK' THEN (
                SELECT NULLIF(string_agg(mapping.display_value, ', ' ORDER BY mapping.bit_index), '')
                FROM plant.register_value_mapping mapping
                WHERE mapping.organization_id = address.organization_id
                  AND mapping.profile_address_id = address.id
                  AND mapping.bit_index IS NOT NULL
                  AND item.value::numeric >= 0
                  AND round(item.value::numeric)::bigint & (1::bigint << mapping.bit_index) <> 0
            )
        END AS display_value
    ) labels ON true
$$;

CREATE OR REPLACE VIEW telemetry.raw_telemetry_latest AS
SELECT
    raw.id AS telemetry_reading_id,
    raw.organization_id,
    raw.plant_id,
    raw.device_id,
    raw.gateway_id,
    raw.observed_at,
    raw.received_at,
    mapped.data_item_map,
    (SELECT count(*)::integer FROM jsonb_object_keys(mapped.data_item_map)) AS parameter_count,
    telemetry.display_data_items(
        raw.organization_id, device.device_model_id, raw.register_address_map
    ) AS display_item_map
FROM telemetry.raw_register_reading_latest raw
JOIN plant.device device
  ON device.organization_id = raw.organization_id
 AND device.plant_id = raw.plant_id
 AND device.id = raw.device_id
CROSS JOIN LATERAL (
    SELECT telemetry.mapped_data_items(
        raw.organization_id, raw.plant_id, raw.device_id,
        device.device_model_id, raw.register_address_map
    ) AS data_item_map
) mapped;
