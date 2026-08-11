ALTER TABLE plant.plant
    ADD COLUMN lifecycle_status text NOT NULL DEFAULT 'OPERATIONAL',
    ADD CONSTRAINT plant_lifecycle_status CHECK (lifecycle_status IN ('IN_CONSTRUCTION', 'OPERATIONAL', 'OFFLINE', 'RETIRED'));
