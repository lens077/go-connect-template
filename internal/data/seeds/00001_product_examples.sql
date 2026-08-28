-- Idempotent example data for local development.

-- +goose Up
INSERT INTO example.products (sku, name, price_cents)
VALUES
    ('DEMO-001', 'Example notebook', 1299),
    ('DEMO-002', 'Example pen', 299)
ON CONFLICT (sku) DO UPDATE SET
    name = EXCLUDED.name,
    price_cents = EXCLUDED.price_cents,
    updated_at = now();

-- +goose Down
DELETE FROM example.products WHERE sku IN ('DEMO-001', 'DEMO-002');
