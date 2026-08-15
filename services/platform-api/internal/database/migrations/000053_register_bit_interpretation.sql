ALTER TABLE plant.register_profile_address
    ADD COLUMN bit_interpretation text NOT NULL DEFAULT 'INDEPENDENT_FLAGS';

ALTER TABLE plant.register_profile_address
    ADD CONSTRAINT register_profile_address_bit_interpretation
    CHECK (bit_interpretation IN ('ONE_HOT', 'INDEPENDENT_FLAGS'));

ALTER TABLE plant.register_profile_address
    DROP CONSTRAINT register_profile_address_alarm_mapping;
