-- Migration: 229_channel_monitor_interval_cap
-- 渠道监控 interval_seconds 上限从 3600 提到 9600，与
-- service.monitorMaxIntervalSeconds / 前端 MAX_INTERVAL_SECONDS 对齐。
-- 125_add_channel_monitors 的 CHECK 不可改（已应用、checksum 锁定），
-- 这里幂等替换约束。

DO $$
DECLARE
    constraint_def TEXT;
BEGIN
    SELECT pg_get_constraintdef(c.oid)
      INTO constraint_def
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
     WHERE t.relname = 'channel_monitors'
       AND c.conname = 'channel_monitors_interval_check';

    IF constraint_def IS NULL OR position('9600' IN constraint_def) = 0 THEN
        ALTER TABLE channel_monitors
            DROP CONSTRAINT IF EXISTS channel_monitors_interval_check;
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_interval_check
            CHECK (interval_seconds BETWEEN 15 AND 9600);
    END IF;
END $$;

COMMENT ON CONSTRAINT channel_monitors_interval_check ON channel_monitors IS
    'interval_seconds must stay in [15, 9600] to match service/frontend caps';
