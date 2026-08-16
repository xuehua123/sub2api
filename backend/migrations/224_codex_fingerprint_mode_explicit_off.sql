SET LOCAL lock_timeout = '5s';

-- Codex fingerprint convergence is explicitly opt-in.  This guard is kept in
-- the database because a blue-green release can have an older writer replace
-- accounts.extra after the newer application has already normalized it.
--
-- Do not "repair" a non-object extra value by replacing it with an object:
-- that could silently discard operator data.  Block the release instead so it
-- can be repaired deliberately before the invariant is enforced.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM accounts
        WHERE platform = 'openai'
          AND type = 'oauth'
          AND deleted_at IS NULL
          AND (
              extra IS NULL
              OR jsonb_typeof(extra) IS DISTINCT FROM 'object'
          )
    ) THEN
        RAISE EXCEPTION
            'live OpenAI OAuth account extra must be a JSON object before enforcing codex_fingerprint_mode'
            USING ERRCODE = '22023';
    END IF;
END;
$$;

CREATE OR REPLACE FUNCTION public.enforce_openai_oauth_codex_fingerprint_mode_explicit_off()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    fingerprint_mode JSONB;
BEGIN
    -- The invariant is intentionally scoped to accounts which can actually
    -- make live Codex OAuth requests.  Deleted or non-OpenAI/non-OAuth rows
    -- retain their existing metadata unchanged.
    IF NEW.platform IS DISTINCT FROM 'openai'
       OR NEW.type IS DISTINCT FROM 'oauth'
       OR NEW.deleted_at IS NOT NULL THEN
        RETURN NEW;
    END IF;

    -- Fail closed rather than turning an array/scalar/null into an object and
    -- silently losing unrelated administrator-owned metadata.
    IF NEW.extra IS NULL
       OR jsonb_typeof(NEW.extra) IS DISTINCT FROM 'object' THEN
        RAISE EXCEPTION
            'live OpenAI OAuth account extra must be a JSON object before enforcing codex_fingerprint_mode'
            USING ERRCODE = '22023';
    END IF;

    fingerprint_mode := NEW.extra->'codex_fingerprint_mode';
    IF jsonb_typeof(fingerprint_mode) = 'string'
       AND fingerprint_mode #>> '{}' IN ('off', 'device', 'session', 'full') THEN
        RETURN NEW;
    END IF;

    NEW.extra := jsonb_set(
        NEW.extra,
        '{codex_fingerprint_mode}',
        '"off"'::jsonb,
        true
    );
    RETURN NEW;
END;
$$;

-- PostgreSQL runs same-kind triggers alphabetically.  Keep this name before
-- accounts_enforce_openai_long_context_billing_extra so malformed non-object
-- metadata fails with this migration's explicit no-data-loss error first.
DROP TRIGGER IF EXISTS accounts_enforce_codex_fingerprint_mode_explicit_off ON accounts;
CREATE TRIGGER accounts_enforce_codex_fingerprint_mode_explicit_off
BEFORE INSERT OR UPDATE OF platform, type, extra, deleted_at
ON accounts
FOR EACH ROW
EXECUTE FUNCTION public.enforce_openai_oauth_codex_fingerprint_mode_explicit_off();

-- Historical account rows are normalized once.  A data-changing CTE gives
-- schedulers a durable account_changed event for exactly the rows adjusted by
-- this backfill; a later migration re-run finds no more invalid object values.
WITH updated_accounts AS (
    UPDATE accounts
    SET extra = jsonb_set(
        extra,
        '{codex_fingerprint_mode}',
        '"off"'::jsonb,
        true
    )
    WHERE platform = 'openai'
      AND type = 'oauth'
      AND deleted_at IS NULL
      AND (
          jsonb_typeof(extra->'codex_fingerprint_mode') IS DISTINCT FROM 'string'
          OR extra->>'codex_fingerprint_mode' NOT IN ('off', 'device', 'session', 'full')
      )
    RETURNING id
)
INSERT INTO scheduler_outbox (event_type, account_id)
SELECT 'account_changed', id
FROM updated_accounts;
