-- Auto-stop needs two below-limit syncs at least 24 hours apart, and waits after the owner resumes by hand.
ALTER TABLE campaigns ADD COLUMN auto_strike_at timestamptz, ADD COLUMN auto_hold_until timestamptz;
