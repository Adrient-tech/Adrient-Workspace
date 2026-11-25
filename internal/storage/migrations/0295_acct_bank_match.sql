-- Wave 6 (fin): match a bank statement line to a journal that is already in the books.
-- A matched line points at the existing journal (status 'matched'); nothing is posted twice.
-- A journal can be claimed by at most one bank line.
CREATE UNIQUE INDEX IF NOT EXISTS acct_bank_lines_journal_uq
    ON adrient.acct_bank_lines (org_id, journal_id)
    WHERE journal_id IS NOT NULL AND status IN ('added','matched');
