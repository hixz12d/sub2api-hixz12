-- 邀请关系的绑定时间：注册绑定和管理员指定/改绑时写入，返利有效期从这里起算。
ALTER TABLE user_affiliates
    ADD COLUMN IF NOT EXISTS inviter_bound_at TIMESTAMPTZ;

COMMENT ON COLUMN user_affiliates.inviter_bound_at IS '当前邀请人的绑定时间；未绑定邀请人时为 NULL';

-- 已有的邀请关系没有记录绑定时间，按档案创建时间回填。
UPDATE user_affiliates
SET inviter_bound_at = created_at
WHERE inviter_id IS NOT NULL
  AND inviter_bound_at IS NULL;
