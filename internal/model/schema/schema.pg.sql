DROP SCHEMA IF EXISTS progstack;

CREATE SCHEMA progstack;

-- Grant usage on the schema
GRANT USAGE ON SCHEMA progstack TO progstack_user;

-- Grant all privileges on existing tables in the schema
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA progstack TO progstack_user;
-- Grant all privileges on existing sequences in the schema
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA progstack TO progstack_user;

GRANT ALL ON SCHEMA progstack TO progstack_user;

-- Set default privileges for future tables created in the schema
ALTER DEFAULT PRIVILEGES IN SCHEMA progstack
	GRANT ALL PRIVILEGES ON TABLES to progstack_user;

-- Set default privileges for future sequences created in the schema
ALTER DEFAULT PRIVILEGES IN SCHEMA progstack
	GRANT ALL PRIVILEGES ON SEQUENCES to progstack_user;

CREATE TABLE users (
	id		SERIAL				PRIMARY KEY,
	email		VARCHAR(255)	NOT NULL	UNIQUE,				-- Github email
	username	VARCHAR(255)	NOT NULL	UNIQUE,				-- GitHub username
	access_tk	TEXT,								-- GitHubApp access token
	created_at	TIMESTAMPTZ	NOT NULL			DEFAULT(now())
);

CREATE TABLE sessions (
	id		SERIAL				PRIMARY KEY,
	token		TEXT		NOT NULL	UNIQUE,
	user_id		INTEGER 	NOT NULL,
	active		BOOLEAN		NOT NULL			DEFAULT(true),
	created_at	TIMESTAMPTZ	NOT NULL			DEFAULT(now()),
	expires_at	TIMESTAMPTZ	NOT NULL			DEFAULT(now() + INTERVAL '1 month'),

	CONSTRAINT fk_user_id
		FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE unauth_sessions (
	id		SERIAL				PRIMARY KEY,
	token		TEXT		NOT NULL	UNIQUE,
	active		BOOLEAN		NOT NULL			DEFAULT(true),
	created_at	TIMESTAMPTZ	NOT NULL			DEFAULT(now()),
	expires_at	TIMESTAMPTZ	NOT NULL			DEFAULT(now() + INTERVAL '7 days')
);
