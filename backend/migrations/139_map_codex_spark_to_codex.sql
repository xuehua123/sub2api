-- Map GPT-5.3 Codex Spark aliases to GPT-5.3 Codex for OpenAI accounts.
--
-- codex2api does not currently support the Spark model family as a separate
-- upstream model. Existing OpenAI accounts use model_mapping as both the
-- routing map and whitelist, so add every supported Spark effort suffix.

UPDATE accounts
SET credentials = jsonb_set(
    COALESCE(credentials, '{}'::jsonb),
    '{model_mapping}',
    COALESCE(credentials->'model_mapping', '{}'::jsonb) || '{
        "gpt-5.3-codex-spark": "gpt-5.3-codex",
        "gpt-5.3-codex-spark-low": "gpt-5.3-codex",
        "gpt-5.3-codex-spark-medium": "gpt-5.3-codex",
        "gpt-5.3-codex-spark-high": "gpt-5.3-codex",
        "gpt-5.3-codex-spark-xhigh": "gpt-5.3-codex"
    }'::jsonb,
    true
),
updated_at = NOW()
WHERE platform = 'openai'
  AND deleted_at IS NULL
  AND credentials->'model_mapping' IS NOT NULL;
