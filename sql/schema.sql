CREATE TABLE IF NOT EXISTS leads (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    source_id TEXT,
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
    performance_score INTEGER,
    seo_score INTEGER,
    accessibility_score INTEGER,
    mobile_ok INTEGER,
    has_cta INTEGER,
    has_booking INTEGER,
    has_https INTEGER,
    data_confidence TEXT DEFAULT 'unknown',
    lead_score INTEGER NOT NULL DEFAULT 0,
    score_reasons TEXT NOT NULL DEFAULT '[]',
    pipeline_status TEXT NOT NULL DEFAULT 'new',
    do_not_contact INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source, source_id)
);
CREATE INDEX IF NOT EXISTS idx_leads_market ON leads(country, city, category);
CREATE INDEX IF NOT EXISTS idx_leads_score ON leads(lead_score DESC);
CREATE INDEX IF NOT EXISTS idx_leads_pipeline ON leads(pipeline_status);

CREATE TABLE IF NOT EXISTS search_runs (
    id TEXT PRIMARY KEY,
    country TEXT,
    city TEXT,
    category TEXT,
    radius_km INTEGER,
    result_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS search_run_leads (
    search_id TEXT NOT NULL,
    lead_id INTEGER NOT NULL,
    PRIMARY KEY(search_id, lead_id)
);
CREATE INDEX IF NOT EXISTS idx_search_run_leads_search ON search_run_leads(search_id);
