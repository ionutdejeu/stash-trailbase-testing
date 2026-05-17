PRAGMA foreign_keys = ON;

CREATE TABLE namespaces (
    id          INTEGER PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;

CREATE TABLE episodes (
    id              INTEGER PRIMARY KEY,
    namespace_id    INTEGER NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    content         TEXT NOT NULL,
    embedding       TEXT,
    embedding_model TEXT,
    occurred_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TEXT
) STRICT;
CREATE INDEX idx_episodes_namespace_occurred_at ON episodes (namespace_id, occurred_at);
CREATE INDEX idx_episodes_namespace_deleted_at ON episodes (namespace_id, deleted_at);

CREATE TABLE facts (
    id              INTEGER PRIMARY KEY,
    namespace_id    INTEGER NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    content         TEXT NOT NULL,
    embedding       TEXT,
    embedding_model TEXT,
    confidence      REAL NOT NULL DEFAULT 1.0 CHECK (confidence >= 0 AND confidence <= 1),
    valid_from      TEXT,
    valid_until     TEXT,
    created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TEXT
) STRICT;
CREATE INDEX idx_facts_namespace_confidence ON facts (namespace_id, confidence);
CREATE INDEX idx_facts_namespace_validity ON facts (namespace_id, valid_from, valid_until);
CREATE INDEX idx_facts_namespace_deleted_at ON facts (namespace_id, deleted_at);

CREATE TABLE fact_sources (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    fact_id     INTEGER NOT NULL REFERENCES facts(id) ON DELETE CASCADE,
    episode_id  INTEGER NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    UNIQUE (fact_id, episode_id)
) STRICT;

CREATE TABLE relationships (
    id              INTEGER PRIMARY KEY,
    namespace_id    INTEGER NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    from_entity     TEXT NOT NULL,
    relation_type   TEXT NOT NULL,
    to_entity       TEXT NOT NULL,
    confidence      REAL NOT NULL DEFAULT 0.5 CHECK (confidence >= 0 AND confidence <= 1),
    source_fact_id  INTEGER REFERENCES facts(id) ON DELETE CASCADE,
    created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TEXT
) STRICT;
CREATE INDEX idx_relationships_namespace_from_entity ON relationships (namespace_id, from_entity);
CREATE INDEX idx_relationships_namespace_to_entity ON relationships (namespace_id, to_entity);
CREATE INDEX idx_relationships_namespace_relation_type ON relationships (namespace_id, relation_type);

CREATE TABLE patterns (
    id              INTEGER PRIMARY KEY,
    namespace_id    INTEGER NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    content         TEXT NOT NULL,
    confidence      REAL NOT NULL DEFAULT 0.5 CHECK (confidence >= 0 AND confidence <= 1),
    source_fact_ids TEXT NOT NULL DEFAULT '[]',
    source_rel_ids  TEXT NOT NULL DEFAULT '[]',
    coherence_score REAL NOT NULL DEFAULT 0.0,
    created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TEXT
) STRICT;
CREATE INDEX idx_patterns_namespace_id ON patterns (namespace_id);

CREATE TABLE contexts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    namespace_id  INTEGER NOT NULL UNIQUE REFERENCES namespaces(id) ON DELETE CASCADE,
    focus         TEXT NOT NULL DEFAULT '',
    expires_at    TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;

CREATE TABLE consolidation_progress (
    id                          INTEGER PRIMARY KEY AUTOINCREMENT,
    namespace_id                INTEGER NOT NULL UNIQUE REFERENCES namespaces(id) ON DELETE CASCADE,
    last_episode_id             INTEGER NOT NULL DEFAULT 0,
    last_fact_id                INTEGER NOT NULL DEFAULT 0,
    last_relationship_id        INTEGER NOT NULL DEFAULT 0,
    last_pattern_fact_id        INTEGER NOT NULL DEFAULT 0,
    last_pattern_rel_id         INTEGER NOT NULL DEFAULT 0,
    last_run                    TEXT,
    updated_at                  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;

CREATE TABLE settings (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    key         TEXT NOT NULL UNIQUE,
    value       TEXT NOT NULL,
    updated_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;

CREATE TABLE embedding_cache (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    text_hash   TEXT NOT NULL,
    model       TEXT NOT NULL,
    text        TEXT NOT NULL,
    embedding   TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (text_hash, model)
) STRICT;

CREATE TABLE contradictions (
    id              INTEGER PRIMARY KEY,
    namespace_id    INTEGER NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    old_fact_id     INTEGER NOT NULL REFERENCES facts(id) ON DELETE CASCADE,
    new_fact_id     INTEGER NOT NULL REFERENCES facts(id) ON DELETE CASCADE,
    entity          TEXT NOT NULL,
    property        TEXT NOT NULL,
    old_value       TEXT NOT NULL,
    new_value       TEXT NOT NULL,
    confidence      REAL NOT NULL DEFAULT 0.5 CHECK (confidence >= 0 AND confidence <= 1),
    method          TEXT NOT NULL DEFAULT 'structured',
    resolved        INTEGER NOT NULL DEFAULT 0 CHECK (resolved IN (0, 1)),
    resolution      TEXT,
    resolved_at     TEXT,
    created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
) STRICT;
CREATE INDEX idx_contradictions_namespace_resolved ON contradictions (namespace_id, resolved);
CREATE INDEX idx_contradictions_old_fact_id ON contradictions (old_fact_id);
CREATE INDEX idx_contradictions_new_fact_id ON contradictions (new_fact_id);

CREATE TABLE causal_links (
    id              INTEGER PRIMARY KEY,
    namespace_id    INTEGER NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    cause_fact_id   INTEGER NOT NULL REFERENCES facts(id) ON DELETE CASCADE,
    effect_fact_id  INTEGER NOT NULL REFERENCES facts(id) ON DELETE CASCADE,
    confidence      REAL NOT NULL DEFAULT 0.5 CHECK (confidence >= 0 AND confidence <= 1),
    method          TEXT NOT NULL DEFAULT 'extracted',
    created_at      TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TEXT
) STRICT;
CREATE UNIQUE INDEX idx_causal_links_unique_active ON causal_links (cause_fact_id, effect_fact_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_causal_links_namespace_cause ON causal_links (namespace_id, cause_fact_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_causal_links_namespace_effect ON causal_links (namespace_id, effect_fact_id) WHERE deleted_at IS NULL;

CREATE TABLE hypotheses (
    id                INTEGER PRIMARY KEY,
    namespace_id      INTEGER NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    content           TEXT NOT NULL,
    confidence        REAL NOT NULL DEFAULT 0.5 CHECK (confidence >= 0 AND confidence <= 1),
    status            TEXT NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed', 'testing', 'confirmed', 'rejected')),
    verification_plan TEXT NOT NULL DEFAULT '',
    method            TEXT NOT NULL DEFAULT 'asserted',
    confirmed_fact_id INTEGER REFERENCES facts(id),
    rejection_reason  TEXT,
    source_fact_ids   TEXT NOT NULL DEFAULT '[]',
    tested_at         TEXT,
    confirmed_at      TEXT,
    rejected_at       TEXT,
    created_at        TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at        TEXT
) STRICT;
CREATE INDEX idx_hypotheses_namespace_status ON hypotheses (namespace_id, status);
CREATE INDEX idx_hypotheses_namespace_deleted_at ON hypotheses (namespace_id, deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_hypotheses_confirmed_fact_id ON hypotheses (confirmed_fact_id) WHERE confirmed_fact_id IS NOT NULL;

CREATE TABLE goals (
    id             INTEGER PRIMARY KEY,
    namespace_id   INTEGER NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    parent_id      INTEGER REFERENCES goals(id) ON DELETE CASCADE,
    content        TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'abandoned')),
    priority       INTEGER NOT NULL DEFAULT 0,
    notes          TEXT NOT NULL DEFAULT '',
    completed_at   TEXT,
    abandoned_at   TEXT,
    created_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at     TEXT
) STRICT;
CREATE INDEX idx_goals_namespace_status ON goals (namespace_id, status);
CREATE INDEX idx_goals_namespace_parent ON goals (namespace_id, parent_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_goals_parent_id ON goals (parent_id) WHERE deleted_at IS NULL;

CREATE TABLE failures (
    id            INTEGER PRIMARY KEY,
    namespace_id  INTEGER NOT NULL REFERENCES namespaces(id) ON DELETE CASCADE,
    goal_id       INTEGER REFERENCES goals(id) ON DELETE SET NULL,
    content       TEXT NOT NULL,
    reason        TEXT NOT NULL DEFAULT '',
    lesson        TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at    TEXT
) STRICT;
CREATE INDEX idx_failures_namespace_deleted_at ON failures (namespace_id, deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_failures_goal_id ON failures (goal_id) WHERE goal_id IS NOT NULL;
