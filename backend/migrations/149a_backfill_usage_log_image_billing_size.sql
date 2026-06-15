-- Normalize legacy image usage rows before entitlement migrations touch usage_logs.
--
-- Migration 136 added NOT VALID image billing constraints. Existing rows may still
-- have image_count > 0 with a NULL/legacy image_size; any later UPDATE on those
-- rows re-checks the constraint and fails. Keep the historical cost/token data
-- unchanged and only fill the billing-size audit fields with a conservative
-- legacy marker.

UPDATE usage_logs
SET
    image_size = 'mixed',
    image_size_source = 'legacy'
WHERE COALESCE(image_count, 0) > 0
  AND (image_size IS NULL OR image_size NOT IN ('1K', '2K', '4K', 'mixed'));
