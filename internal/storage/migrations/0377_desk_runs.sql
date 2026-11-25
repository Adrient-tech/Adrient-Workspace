-- Service desk: record which time-based automation already ran on which ticket (once per
-- ticket), plus a per-row rule sweep index. Idempotent.
CREATE TABLE IF NOT EXISTS adrient.ticket_automation_runs (
    automation_id UUID NOT NULL REFERENCES adrient.ticket_automations(id) ON DELETE CASCADE,
    ticket_id     UUID NOT NULL REFERENCES adrient.tickets(id) ON DELETE CASCADE,
    ran_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (automation_id, ticket_id)
);
CREATE INDEX IF NOT EXISTS tickets_snooze_idx ON adrient.tickets (snoozed_until) WHERE snoozed_until IS NOT NULL;
CREATE INDEX IF NOT EXISTS tickets_open_updated_idx ON adrient.tickets (project_id, updated_at)
    WHERE status NOT IN ('resolved','closed');
