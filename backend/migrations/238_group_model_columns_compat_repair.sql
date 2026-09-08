-- Expand before 235 can rename the legacy column; also replayed by 238 for
-- databases that already applied 235-237. Keep both copies identical.
-- The runner executes each file in a transaction. Never rewrite 235-237.
SET LOCAL lock_timeout = '5s';
LOCK TABLE groups IN ACCESS EXCLUSIVE MODE;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS model_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Do not silently choose between conflicting nonempty configurations.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM groups
        WHERE COALESCE(model_allowlist, '{}'::jsonb) <> '{}'::jsonb
          AND COALESCE(models_list_config, '{}'::jsonb) <> '{}'::jsonb
          AND model_allowlist IS DISTINCT FROM models_list_config) THEN
        RAISE EXCEPTION 'Conflicting group model configurations; reconcile before release';
    END IF;
END
$$;

DROP TRIGGER IF EXISTS trg_sync_group_model_allowlist_columns ON groups;
UPDATE groups
SET model_allowlist = COALESCE(NULLIF(model_allowlist, '{}'::jsonb), models_list_config, '{}'::jsonb),
    models_list_config = COALESCE(NULLIF(model_allowlist, '{}'::jsonb), models_list_config, '{}'::jsonb)
WHERE model_allowlist IS DISTINCT FROM models_list_config
   OR model_allowlist IS NULL OR models_list_config IS NULL;
ALTER TABLE groups ALTER COLUMN model_allowlist SET DEFAULT '{}'::jsonb;
ALTER TABLE groups ALTER COLUMN model_allowlist SET NOT NULL;
ALTER TABLE groups ALTER COLUMN models_list_config SET DEFAULT '{}'::jsonb;
ALTER TABLE groups ALTER COLUMN models_list_config SET NOT NULL;

CREATE OR REPLACE FUNCTION sync_group_model_columns_compat()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        NEW.model_allowlist := COALESCE(NEW.model_allowlist, '{}'::jsonb);
        NEW.models_list_config := COALESCE(NEW.models_list_config, '{}'::jsonb);
        IF NEW.model_allowlist = '{}'::jsonb THEN
            NEW.model_allowlist := NEW.models_list_config;
        ELSIF NEW.models_list_config = '{}'::jsonb THEN
            NEW.models_list_config := NEW.model_allowlist;
        END IF;
    ELSE
        IF NEW.model_allowlist IS DISTINCT FROM OLD.model_allowlist
           AND NEW.models_list_config IS NOT DISTINCT FROM OLD.models_list_config THEN
            NEW.models_list_config := NEW.model_allowlist;
        ELSIF NEW.models_list_config IS DISTINCT FROM OLD.models_list_config
           AND NEW.model_allowlist IS NOT DISTINCT FROM OLD.model_allowlist THEN
            NEW.model_allowlist := NEW.models_list_config;
        END IF;
    END IF;
    IF NEW.model_allowlist IS DISTINCT FROM NEW.models_list_config THEN
        RAISE EXCEPTION 'Conflicting group model configurations in one write'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$$;
DROP TRIGGER IF EXISTS group_model_columns_compat ON groups;
CREATE TRIGGER group_model_columns_compat
BEFORE INSERT OR UPDATE ON groups
FOR EACH ROW EXECUTE FUNCTION sync_group_model_columns_compat();
