package logbook

const schema = `
CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS counters(id INTEGER PRIMARY KEY,request_count INTEGER NOT NULL DEFAULT 0,request_bytes INTEGER NOT NULL DEFAULT 0,event_count INTEGER NOT NULL DEFAULT 0,event_bytes INTEGER NOT NULL DEFAULT 0,dropped INTEGER NOT NULL DEFAULT 0,failures INTEGER NOT NULL DEFAULT 0);
INSERT OR IGNORE INTO counters(id) VALUES(1);
CREATE TABLE IF NOT EXISTS requests(seq INTEGER PRIMARY KEY AUTOINCREMENT,id TEXT NOT NULL UNIQUE,created_at INTEGER NOT NULL,status TEXT NOT NULL,source TEXT NOT NULL,model TEXT NOT NULL,provider TEXT NOT NULL,key_id TEXT NOT NULL,http INTEGER,error_code TEXT NOT NULL,stage TEXT NOT NULL,size INTEGER NOT NULL,document TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS request_time ON requests(created_at);
CREATE INDEX IF NOT EXISTS request_status ON requests(status,seq DESC);
CREATE INDEX IF NOT EXISTS request_source ON requests(source,seq DESC);
CREATE INDEX IF NOT EXISTS request_model ON requests(model,seq DESC);
CREATE INDEX IF NOT EXISTS request_provider ON requests(provider,seq DESC);
CREATE INDEX IF NOT EXISTS request_key ON requests(key_id,seq DESC);
CREATE INDEX IF NOT EXISTS request_code ON requests(error_code,seq DESC);
CREATE INDEX IF NOT EXISTS request_stage ON requests(stage,seq DESC);
CREATE INDEX IF NOT EXISTS request_http ON requests(http,seq DESC);
CREATE INDEX IF NOT EXISTS request_agent ON requests(COALESCE(json_extract(document,'$.agent'),''),seq DESC);
CREATE INDEX IF NOT EXISTS request_binding ON requests(COALESCE(json_extract(document,'$.binding'),''),seq DESC);
CREATE INDEX IF NOT EXISTS request_project ON requests(COALESCE(json_extract(document,'$.project'),''),seq DESC);
CREATE INDEX IF NOT EXISTS request_session ON requests(COALESCE(json_extract(document,'$.session'),''),seq DESC);
CREATE TABLE IF NOT EXISTS events(seq INTEGER PRIMARY KEY AUTOINCREMENT,id TEXT NOT NULL UNIQUE,created_at INTEGER NOT NULL,level TEXT NOT NULL,source TEXT NOT NULL,event TEXT NOT NULL,request_id TEXT NOT NULL,size INTEGER NOT NULL,document TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS event_time ON events(created_at);
CREATE INDEX IF NOT EXISTS event_level ON events(level,seq DESC);
CREATE INDEX IF NOT EXISTS event_source ON events(source,seq DESC);
CREATE INDEX IF NOT EXISTS event_name ON events(event,seq DESC);
CREATE TRIGGER IF NOT EXISTS request_insert AFTER INSERT ON requests BEGIN UPDATE counters SET request_count=request_count+1,request_bytes=request_bytes+NEW.size WHERE id=1; END;
CREATE TRIGGER IF NOT EXISTS request_update AFTER UPDATE ON requests BEGIN UPDATE counters SET request_bytes=request_bytes+NEW.size-OLD.size WHERE id=1; END;
CREATE TRIGGER IF NOT EXISTS request_delete AFTER DELETE ON requests BEGIN UPDATE counters SET request_count=request_count-1,request_bytes=request_bytes-OLD.size WHERE id=1; END;
CREATE TRIGGER IF NOT EXISTS event_insert AFTER INSERT ON events BEGIN UPDATE counters SET event_count=event_count+1,event_bytes=event_bytes+NEW.size WHERE id=1; END;
CREATE TRIGGER IF NOT EXISTS event_delete AFTER DELETE ON events BEGIN UPDATE counters SET event_count=event_count-1,event_bytes=event_bytes-OLD.size WHERE id=1; END;
PRAGMA user_version=1;
`
