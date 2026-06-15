ALTER TYPE step_status ADD VALUE IF NOT EXISTS 'init';
ALTER TYPE step_status ADD VALUE IF NOT EXISTS 'cancelled';

ALTER TABLE steps ALTER COLUMN status SET DEFAULT 'init';
