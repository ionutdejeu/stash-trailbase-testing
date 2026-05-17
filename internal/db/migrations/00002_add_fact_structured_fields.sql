-- +goose Up
ALTER TABLE facts ADD COLUMN entity TEXT;
ALTER TABLE facts ADD COLUMN property TEXT;
ALTER TABLE facts ADD COLUMN value TEXT;
CREATE INDEX idx_facts_namespace_entity_property_active ON facts (namespace_id, entity, property) WHERE entity IS NOT NULL AND valid_until IS NULL;