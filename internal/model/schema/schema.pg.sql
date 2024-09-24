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
	gh_user_id	BIGINT		NOT NULL,
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
	expires_at	TIMESTAMPTZ	NOT NULL			DEFAULT(now() + INTERVAL '1 month'), -- XXX: set from configuration on creation

	CONSTRAINT fk_user_id
		FOREIGN KEY (user_id)
		REFERENCES users(id)
		ON DELETE CASCADE -- delete sessions if user deleted
);

CREATE TABLE unauth_sessions (
	id		SERIAL				PRIMARY KEY,
	token		TEXT		NOT NULL	UNIQUE,
	created_at	TIMESTAMPTZ	NOT NULL			DEFAULT(now()),
	expires_at	TIMESTAMPTZ	NOT NULL			DEFAULT(now() + INTERVAL '7 days'), -- XXX: set from configuration on creation
	active		BOOLEAN		NOT NULL			DEFAULT(true)
);

CREATE TABLE installations (
	id			SERIAL					PRIMARY KEY,
	gh_installation_id	BIGINT			NOT NULL	UNIQUE,
	user_id			INTEGER			NOT NULL,
	active			BOOLEAN			NOT NULL			DEFAULT(true),
	created_at		TIMESTAMPTZ		NOT NULL			DEFAULT(now()),
	deleted_at		TIMESTAMPTZ,

	CONSTRAINT fk_user_id
		FOREIGN KEY (user_id)
		REFERENCES users(id)
		ON DELETE CASCADE -- delete installations if user deleted
);

CREATE TABLE blogs (
	id 			SERIAL				PRIMARY KEY,
	gh_repository_id	BIGINT		NOT NULL,
	installation_id		INTEGER		NOT NULL,
	name			VARCHAR(255)	NOT NULL,
	full_name		VARCHAR(255)	NOT NULL,			-- needed for path construction
	url			VARCHAR(255)	NOT NULL,
	active			BOOLEAN		NOT NULL			DEFAULT(true),
	created_at		TIMESTAMPTZ	NOT NULL			DEFAULT(now()),
	updated_at		TIMESTAMPTZ	NOT NULL			DEFAULT(now()),

	CONSTRAINT fk_installation_id
		FOREIGN KEY (installation_id)
		REFERENCES installations(id)
		ON DELETE CASCADE -- delete repositories when installation deleted
);
