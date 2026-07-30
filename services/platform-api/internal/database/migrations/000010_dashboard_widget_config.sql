ALTER TABLE user_dashboard
    ADD COLUMN widget_configs jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN published_widget_configs jsonb;

UPDATE user_dashboard
SET published_widget_configs = '{}'::jsonb
WHERE published_layouts IS NOT NULL;

ALTER TABLE user_dashboard
    ADD CONSTRAINT user_dashboard_widget_configs_object CHECK (jsonb_typeof(widget_configs) = 'object'),
    ADD CONSTRAINT user_dashboard_published_widget_configs_state CHECK (
        (published_layouts IS NULL AND published_widget_configs IS NULL)
        OR
        (published_layouts IS NOT NULL AND jsonb_typeof(published_widget_configs) = 'object')
    );
