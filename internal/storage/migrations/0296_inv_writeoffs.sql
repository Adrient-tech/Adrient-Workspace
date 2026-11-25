-- Wave 6 (fin): why stock was removed. Write-offs for damaged / expired / lost / internal use
-- record a loss reason so the shrinkage report can add them up. Existing rows keep ''.
ALTER TABLE adrient.inv_history ADD COLUMN IF NOT EXISTS loss_reason TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS inv_history_loss_idx ON adrient.inv_history (org_id, created_at DESC) WHERE loss_reason <> '';
