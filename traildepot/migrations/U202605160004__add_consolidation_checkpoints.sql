ALTER TABLE consolidation_progress ADD COLUMN last_goal_progress_fact_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE consolidation_progress ADD COLUMN last_failure_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE consolidation_progress ADD COLUMN last_failure_episode_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE consolidation_progress ADD COLUMN last_hypothesis_fact_id INTEGER NOT NULL DEFAULT 0;
