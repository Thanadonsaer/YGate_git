ALTER TABLE plant
    ADD COLUMN image_url text;

COMMENT ON COLUMN plant.image_url IS 'Relative API path for the Plant primary image';