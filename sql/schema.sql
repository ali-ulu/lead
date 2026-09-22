CREATE TABLE IF NOT EXISTS leads (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    source_id TEXT,
    source_refs TEXT NOT NULL DEFAULT '{}',
    name TEXT NOT NULL,
    country TEXT,
    city TEXT,
    category TEXT,
    latitude REAL,
    longitude REAL,
    website TEXT,
    phone TEXT,
    email TEXT,
    social_url TEXT,
    social_links TEXT NOT NULL DEFAULT '{}',
    website_status TEXT NOT NULL DEFAULT 'unknown',
    verification_status TEXT NOT NULL DEFAULT 'unverified',
    verification_notes TEXT NOT NULL DEFAULT '[]',
    performance_score INTEGER,
    seo_score INTEGER,
    accessibility_score INTEGER,
    mobile_ok INTEGER,
    has_cta INTEGER,
    has_booking INTEGER,
    has_https INTEGER,
    audit_engine TEXT,
    rating REAL,
    review_count INTEGER,
    data_confidence TEXT DEFAULT 'unknown',
    contactability_score INTEGER NOT NULL DEFAULT 0,
    commercial_score INTEGER NOT NULL DEFAULT 0,
    intelligence_reasons TEXT NOT NULL DEFAULT '[]',
    lead_score INTEGER NOT NULL DEFAULT 0,
    score_reasons TEXT NOT NULL DEFAULT '[]',
    pipeline_status TEXT NOT NULL DEFAULT 'new',
    engagement_status TEXT NOT NULL DEFAULT 'not_contacted',
    last_contacted_at TEXT,
    last_reply_at TEXT,
    follow_up_at TEXT,
    notes TEXT,
    do_not_contact INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source, source_id)
);
CREATE INDEX IF NOT EXISTS idx_leads_market ON leads(country, city, category);
CREATE INDEX IF NOT EXISTS idx_leads_score ON leads(lead_score DESC);
CREATE INDEX IF NOT EXISTS idx_leads_pipeline ON leads(pipeline_status);
CREATE INDEX IF NOT EXISTS idx_leads_engagement ON leads(engagement_status);

CREATE TABLE IF NOT EXISTS search_runs (
    id TEXT PRIMARY KEY,
    country TEXT,
    city TEXT,
    category TEXT,
    radius_km INTEGER,
    result_count INTEGER NOT NULL DEFAULT 0,
    provider_summary TEXT NOT NULL DEFAULT '{}',
    partial INTEGER NOT NULL DEFAULT 0,
    warnings TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS search_run_leads (
    search_id TEXT NOT NULL,
    lead_id INTEGER NOT NULL,
    PRIMARY KEY(search_id, lead_id)
);
CREATE INDEX IF NOT EXISTS idx_search_run_leads_search ON search_run_leads(search_id);

CREATE TABLE IF NOT EXISTS lead_activities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    lead_id INTEGER NOT NULL,
    kind TEXT NOT NULL,
    channel TEXT,
    status TEXT,
    direction TEXT,
    body TEXT,
    external_id TEXT,
    metadata TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_activities_lead ON lead_activities(lead_id, created_at DESC);

CREATE TABLE IF NOT EXISTS oauth_connections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL,
    account_id TEXT NOT NULL,
    account_name TEXT,
    access_token TEXT NOT NULL,
    token_expires_at TEXT,
    metadata TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, account_id)
);

CREATE TABLE IF NOT EXISTS oauth_states (
    state TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS agent_jobs (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    request_json TEXT NOT NULL,
    result_json TEXT,
    error TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
