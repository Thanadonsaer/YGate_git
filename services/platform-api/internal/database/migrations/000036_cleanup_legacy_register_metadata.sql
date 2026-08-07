-- Remove legacy auto-created rows whose display name was only the wire address,
-- then make the canonical reg-prefixed catalog entries ingestible.
DELETE FROM plant.device_model_register_metadata
WHERE btrim(display_name) = btrim(address_key);

UPDATE plant.device_model_register_metadata
SET is_enabled = true,
    updated_at = now()
WHERE address_key LIKE 'reg%'
  AND is_enabled = false;