-- Example product catalog schema. Production services should use one schema per bounded context.

-- +goose Up
CREATE SCHEMA IF NOT EXISTS example;

CREATE TABLE IF NOT EXISTS example.products (
    id          BIGSERIAL PRIMARY KEY,
    sku         TEXT          NOT NULL UNIQUE,
    name        TEXT          NOT NULL,
    price_cents BIGINT        NOT NULL CHECK (price_cents >= 0),
    active      BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_example_products_active
    ON example.products (active);

-- +goose Down
DROP TABLE IF EXISTS example.products;
DROP SCHEMA IF EXISTS example;
