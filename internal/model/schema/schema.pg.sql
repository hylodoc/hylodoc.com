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
	id			SERIAL				PRIMARY KEY,
	username		VARCHAR(255)	NOT NULL	UNIQUE,
	email			VARCHAR(255)	NOT NULL	UNIQUE,		-- Login email
	created_at		TIMESTAMPTZ	NOT NULL			DEFAULT(now()),
	updated_at		TIMESTAMPTZ	NOT NULL			DEFAULT(now())
);

-- users can link github accounts

CREATE TABLE github_accounts (
	id		SERIAL				PRIMARY KEY,
	user_id		INTEGER		NOT NULL,
	gh_user_id	BIGINT		NOT NULL	UNIQUE,				-- Github userID
	gh_email	VARCHAR(255)	NOT NULL	UNIQUE,				-- Github email
	gh_username	VARCHAR(255)	NOT NULL	UNIQUE,				-- GitHub username
	created_at	TIMESTAMPTZ	NOT NULL			DEFAULT(now()),

	CONSTRAINT fk_user_id
		FOREIGN KEY (user_id)
		REFERENCES users(id)
		ON DELETE CASCADE -- delete github account info if user info deleted
);

-- magic links

CREATE TYPE link_type AS ENUM ('register', 'login');

CREATE TABLE magic (
	id		SERIAL				PRIMARY KEY,
	token		TEXT		NOT NULL	UNIQUE,
	email		VARCHAR(255)	NOT NULL,
	link_type	link_type	NOT NULL,
	active		BOOLEAN		NOT NULL			DEFAULT(true),
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

CREATE TYPE blog_status AS ENUM ('live', 'offline');
CREATE TYPE blog_type AS ENUM ('repository', 'folder');

CREATE TABLE blogs (
	id 			SERIAL				PRIMARY KEY,
	user_id			INTEGER		NOT NULL,
	installation_id		INTEGER		NOT NULL,
	gh_repository_id	BIGINT		NOT NULL	UNIQUE,
	gh_url			VARCHAR(255)	NOT NULL,
	gh_name			VARCHAR(255)	NOT NULL,
	gh_full_name		VARCHAR(255)	NOT NULL,				-- needed for path construction
	subdomain		VARCHAR(255)			UNIQUE,
	demo_subdomain		VARCHAR(255)	NOT NULL	UNIQUE,
	from_address		VARCHAR(255)	NOT NULL,
	blog_type		blog_type	NOT NULL,
	status			blog_status	NOT NULL			DEFAULT('offline'),
	created_at		TIMESTAMPTZ	NOT NULL			DEFAULT(now()),
	updated_at		TIMESTAMPTZ	NOT NULL			DEFAULT(now()),

	CONSTRAINT fk_user_id
		FOREIGN KEY (user_id)
		REFERENCES users(id)
		ON DELETE CASCADE, -- delete repositories when installation deleted

	CONSTRAINT fk_installation_id
		FOREIGN KEY (installation_id)
		REFERENCES installations(id)
		ON DELETE CASCADE -- delete repositories when installation deleted
);

-- blog subscriber lists

CREATE TYPE subscription_status AS ENUM ('active', 'unsubscribed');

CREATE TABLE subscribers (
	id			SERIAL					PRIMARY KEY,
	blog_id			INTEGER			NOT NULL,
	email			VARCHAR(255)		NOT NULL,
	unsubscribe_token 	VARCHAR(255)		NOT NULL,
	status			subscription_status	NOT NULL			DEFAULT('active'),

	created_at		TIMESTAMPTZ		NOT NULL			DEFAULT(now()),

	CONSTRAINT fk_blog_id
		FOREIGN KEY (blog_id)
		REFERENCES blogs(id)
		ON DELETE CASCADE -- delete subscribers when blog deleted
);

--  active subscriber emails should be unique
CREATE UNIQUE INDEX unique_active_subscriber_email
	ON subscribers (email) 
	WHERE status = 'active';

-- stripe integration

CREATE TYPE checkout_session_status AS ENUM ('pending', 'completed');

CREATE TABLE stripe_checkout_sessions (
	stripe_session_id	VARCHAR(255)		NOT NULL	PRIMARY KEY,
	user_id			INTEGER			NOT NULL,
	status			checkout_session_status	NOT NULL	DEFAULT('pending'),
	created_at		TIMESTAMPTZ		NOT NULL	DEFAULT(now()),
	updated_at		TIMESTAMPTZ		NOT NULL	DEFAULT(now()),

	CONSTRAINT fk_user_id
		FOREIGN KEY (user_id)
		REFERENCES users(id)
		ON DELETE CASCADE -- delete checkout sessions when user deleted
);

CREATE TABLE stripe_subscriptions (
	id			SERIAL						PRIMARY KEY,
	user_id			INTEGER				NOT NULL,
	stripe_subscription_id	VARCHAR(255)			NOT NULL	UNIQUE,
	stripe_customer_id	VARCHAR(255)			NOT NULL,
	stripe_price_id		VARCHAR(255)			NOT NULL,
	status			VARCHAR(255)			NOT NULL,
	amount			BIGINT				NOT NULL,
	current_period_start	TIMESTAMPTZ			NOT NULL,
	current_period_end	TIMESTAMPTZ			NOT NULL,
	created_at		TIMESTAMPTZ			NOT NULL	DEFAULT(now()),
	updated_at		TIMESTAMPTZ			NOT NULL	DEFAULT(now())
);
