-- The daily purchase re-check takes the rows it asked about longest ago first, so stuck rows can't starve new ones.
ALTER TABLE purchases ADD COLUMN IF NOT EXISTS checked_at timestamptz;
