-- Harden nullable plan associations without rewriting migration 238.
-- MATCH SIMPLE skips a composite FK when any referencing column is NULL.
-- Source is always required; platform plans additionally bind policy/revision.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'llm_detector_plans'::regclass AND conname = 'llm_detector_plans_id_source_uq') THEN
        ALTER TABLE llm_detector_plans ADD CONSTRAINT llm_detector_plans_id_source_uq UNIQUE (id, source);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'llm_detector_plans'::regclass AND conname = 'llm_detector_plans_id_policy_revision_uq') THEN
        ALTER TABLE llm_detector_plans ADD CONSTRAINT llm_detector_plans_id_policy_revision_uq UNIQUE (id, policy_id, evaluation_revision);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'monitor_jobs'::regclass AND conname = 'monitor_jobs_plan_source_fk') THEN
        ALTER TABLE monitor_jobs ADD CONSTRAINT monitor_jobs_plan_source_fk
            FOREIGN KEY (plan_id, source) REFERENCES llm_detector_plans(id, source) ON DELETE RESTRICT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'monitor_jobs'::regclass AND conname = 'monitor_jobs_plan_policy_revision_fk') THEN
        ALTER TABLE monitor_jobs ADD CONSTRAINT monitor_jobs_plan_policy_revision_fk
            FOREIGN KEY (plan_id, policy_id, evaluation_revision)
            REFERENCES llm_detector_plans(id, policy_id, evaluation_revision) ON DELETE RESTRICT;
    END IF;
END $$;
