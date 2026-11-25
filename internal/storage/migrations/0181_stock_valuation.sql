-- 0181: Valuation (FIFO / weighted average cost layers), tracking mode, value ledger.

ALTER TABLE adrient.inv_products ADD COLUMN IF NOT EXISTS valuation_method TEXT NOT NULL DEFAULT 'avg';
ALTER TABLE adrient.inv_products ADD COLUMN IF NOT EXISTS tracking TEXT NOT NULL DEFAULT 'none';
ALTER TABLE adrient.inv_products ADD COLUMN IF NOT EXISTS shelf_life_days INT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS adrient.inv_cost_layers (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id     UUID NOT NULL REFERENCES adrient.inv_products(id) ON DELETE CASCADE,
    received_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    qty_in         NUMERIC(14,3) NOT NULL,
    qty_remaining  NUMERIC(14,3) NOT NULL,
    unit_cost      NUMERIC(16,4) NOT NULL DEFAULT 0,
    source         TEXT NOT NULL DEFAULT 'receive',
    ref            TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS inv_cost_layers_open_idx ON adrient.inv_cost_layers (product_id, received_at) WHERE qty_remaining > 0;
CREATE INDEX IF NOT EXISTS inv_cost_layers_org_idx ON adrient.inv_cost_layers (org_id);

-- One row per valued movement. value is signed (+ in, - out). journal_state tracks
-- hand-over to Accounting: none (not accounted), pending, posted, failed.
CREATE TABLE IF NOT EXISTS adrient.inv_value_ledger (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES adrient.orgs(id) ON DELETE CASCADE,
    product_id     UUID NOT NULL REFERENCES adrient.inv_products(id) ON DELETE CASCADE,
    location_id    UUID REFERENCES adrient.inv_locations(id) ON DELETE SET NULL,
    ts             TIMESTAMPTZ NOT NULL DEFAULT now(),
    qty            NUMERIC(14,3) NOT NULL,
    unit_cost      NUMERIC(16,4) NOT NULL DEFAULT 0,
    value          NUMERIC(16,2) NOT NULL,
    reason         TEXT NOT NULL,
    ref            TEXT NOT NULL DEFAULT '',
    journal_kind   TEXT NOT NULL DEFAULT '',
    journal_state  TEXT NOT NULL DEFAULT 'none',
    journal_error  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS inv_value_ledger_prod_idx ON adrient.inv_value_ledger (org_id, product_id, ts);
CREATE INDEX IF NOT EXISTS inv_value_ledger_pending_idx ON adrient.inv_value_ledger (org_id) WHERE journal_state IN ('pending','failed');

-- Opening layers for stock that existed before valuation was introduced.
INSERT INTO adrient.inv_cost_layers (org_id, product_id, received_at, qty_in, qty_remaining, unit_cost, source, ref)
SELECT p.org_id, p.id, p.created_at, s.q, s.q, p.cost, 'opening', ''
FROM adrient.inv_products p
JOIN (SELECT product_id, sum(qty) q FROM adrient.inv_stock GROUP BY product_id HAVING sum(qty) > 0) s ON s.product_id = p.id
WHERE NOT EXISTS (SELECT 1 FROM adrient.inv_cost_layers l WHERE l.product_id = p.id);

INSERT INTO adrient.inv_value_ledger (org_id, product_id, ts, qty, unit_cost, value, reason, ref)
SELECT l.org_id, l.product_id, l.received_at, l.qty_in, l.unit_cost, round(l.qty_in * l.unit_cost, 2), 'opening', ''
FROM adrient.inv_cost_layers l
WHERE l.source = 'opening'
  AND NOT EXISTS (SELECT 1 FROM adrient.inv_value_ledger v WHERE v.product_id = l.product_id AND v.reason = 'opening');
