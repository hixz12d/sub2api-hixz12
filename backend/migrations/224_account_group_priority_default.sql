-- New account-group bindings should start in the highest scheduling tier.
-- Existing priorities are intentionally preserved; administrators may have
-- configured them explicitly.
ALTER TABLE account_groups
    ALTER COLUMN priority SET DEFAULT 1;
