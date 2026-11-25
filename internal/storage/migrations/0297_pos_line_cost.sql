-- Wave 6 (fin): the cost of goods is frozen on each POS sale line when the sale completes,
-- so a later supplier price change cannot rewrite what earlier sales earned.
-- NULL means the cost was never recorded (older sales, one-off items).
ALTER TABLE adrient.pos_sale_lines ADD COLUMN IF NOT EXISTS unit_cost NUMERIC(16,4);
ALTER TABLE adrient.pos_sale_lines ADD COLUMN IF NOT EXISTS cost_total NUMERIC(16,2);
