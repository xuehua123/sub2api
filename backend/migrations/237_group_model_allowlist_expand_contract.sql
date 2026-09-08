-- 237: 蓝绿发布兼容迁移。
-- 新旧应用在同一数据库并存期间同时保留两个列，并在单列更新时同步另一列。
DO $$
BEGIN
    IF to_regclass('groups') IS NULL THEN
        RETURN;
    END IF;
    ALTER TABLE groups ADD COLUMN IF NOT EXISTS model_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb;
    ALTER TABLE groups ADD COLUMN IF NOT EXISTS models_list_config JSONB NOT NULL DEFAULT '{}'::jsonb;
    UPDATE groups
       SET model_allowlist = models_list_config
     WHERE model_allowlist = '{}'::jsonb AND models_list_config <> '{}'::jsonb;
END
$$;

CREATE OR REPLACE FUNCTION sync_group_model_allowlist_columns()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.model_allowlist IS DISTINCT FROM OLD.model_allowlist
       AND NEW.models_list_config IS NOT DISTINCT FROM OLD.models_list_config THEN
        NEW.models_list_config := NEW.model_allowlist;
    ELSIF NEW.models_list_config IS DISTINCT FROM OLD.models_list_config
       AND NEW.model_allowlist IS NOT DISTINCT FROM OLD.model_allowlist THEN
        NEW.model_allowlist := NEW.models_list_config;
    END IF;
    RETURN NEW;
END
$$;

DROP TRIGGER IF EXISTS trg_sync_group_model_allowlist_columns ON groups;
CREATE TRIGGER trg_sync_group_model_allowlist_columns
BEFORE UPDATE OF model_allowlist, models_list_config ON groups
FOR EACH ROW EXECUTE FUNCTION sync_group_model_allowlist_columns();
