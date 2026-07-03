-- Allow cyber_policy usage rows to persist request_type=4.
-- The application enum already includes RequestTypeCyberBlocked = 4; older
-- databases may still have the original 0..3 CHECK constraint from migration 061.
DO $$
DECLARE
    constraint_definition text;
BEGIN
    SELECT pg_get_constraintdef(oid)
    INTO constraint_definition
    FROM pg_constraint
    WHERE conrelid = 'usage_logs'::regclass
      AND conname = 'usage_logs_request_type_check';

    IF constraint_definition IS NULL OR constraint_definition NOT LIKE '%4%' THEN
        ALTER TABLE usage_logs DROP CONSTRAINT IF EXISTS usage_logs_request_type_check;
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_request_type_check
            CHECK (request_type IN (0, 1, 2, 3, 4)) NOT VALID;
    END IF;
END $$;
