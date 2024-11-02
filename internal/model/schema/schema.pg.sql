DROP SCHEMA IF EXISTS progstack;

CREATE SCHEMA progstack;

-- UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

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
	gh_awaiting_update	BOOLEAN		NOT NULL			DEFAULT(false),
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

CREATE TABLE auth_sessions (
	id		UUID				PRIMARY KEY	DEFAULT(uuid_generate_v4()),
	user_id		INTEGER		NOT NULL,
	created_at	TIMESTAMPTZ	NOT NULL			DEFAULT(now()),
	expires_at	TIMESTAMPTZ	NOT NULL			DEFAULT(now() + INTERVAL '1 month'), -- XXX: set from configuration on creation
	active		BOOLEAN		NOT NULL			DEFAULT(true),

	CONSTRAINT fk_user_id
		FOREIGN KEY (user_id)
		REFERENCES users(id)
		ON DELETE CASCADE -- delete sessions if user deleted
);

CREATE TABLE unauth_sessions (
	id		UUID				PRIMARY KEY	DEFAULT(uuid_generate_v4()),
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

CREATE TABLE repositories (
	id			SERIAL				PRIMARY KEY,
	installation_id		BIGINT		NOT NULL,
	repository_id		BIGINT		NOT NULL	UNIQUE,
	name			VARCHAR(255)	NOT NULL,
	full_name		VARCHAR(255)	NOT NULL,
	url			VARCHAR(255)	NOT NULL,
	created_at		TIMESTAMPTZ	NOT NULL			DEFAULT(now()),

	CONSTRAINT fk_gh_installation_id
		FOREIGN KEY (installation_id)
		REFERENCES installations(gh_installation_id)
		ON DELETE CASCADE -- delete repositories when installation deleted
);

CREATE TYPE blog_type AS ENUM ('repository', 'folder');
CREATE TYPE blog_theme AS ENUM ('lit', 'latex');

CREATE TABLE blogs (
	id 			SERIAL				PRIMARY KEY,
	user_id			INTEGER		NOT NULL,
	gh_repository_id	BIGINT				UNIQUE		DEFAULT(NULL),
	gh_url			VARCHAR(255)			UNIQUE		DEFAULT(NULL),
	repository_path		VARCHAR(255)	NOT NULL,			-- path on disk
	theme			blog_theme	NOT NULL			DEFAULT('lit'),
	test_branch		VARCHAR(255),
	live_branch		VARCHAR(255),
	subdomain		VARCHAR(255)	NOT NULL	UNIQUE,
	from_address		VARCHAR(255)	NOT NULL,
	blog_type		blog_type	NOT NULL,
	created_at		TIMESTAMPTZ	NOT NULL			DEFAULT(now()),
	updated_at		TIMESTAMPTZ	NOT NULL			DEFAULT(now()),

	CONSTRAINT fk_repository_id
		FOREIGN KEY (gh_repository_id)
		REFERENCES repositories(repository_id)
		ON DELETE CASCADE, -- delete blogs when repository deleted

	CONSTRAINT blog_type_check CHECK (
		(
			blog_type = 'repository'
			AND gh_repository_id IS NOT NULL
			AND gh_url IS NOT NULL
			AND test_branch IS NOT NULL
			AND live_branch IS NOT NULL
		) OR (
			blog_type = 'folder'
			AND gh_repository_id IS NULL
			AND gh_url IS NULL
			AND test_branch IS NULL
			AND live_branch IS NULL
		)
	)
);

CREATE TABLE generations (
	id		SERIAL		PRIMARY KEY,
	blog		INTEGER		NOT NULL	REFERENCES blogs,
	created_at	TIMESTAMPTZ	NOT NULL	DEFAULT(now()),
	active		BOOLEAN		NOT NULL	DEFAULT(true)
);
CREATE INDEX ON generations(blog);
CREATE UNIQUE INDEX ON generations(blog) WHERE active = true;
CREATE INDEX ON generations(created_at);

CREATE TABLE bindings (
	gen	INTEGER		NOT NULL	REFERENCES generations,
	url	VARCHAR(1000)	NOT NULL,
	file	VARCHAR(1000)	NOT NULL,

	PRIMARY KEY (gen, url)
);

CREATE TABLE _r_posts (
	url		VARCHAR(1000)	NOT NULL,
	blog		INTEGER		NOT NULL	REFERENCES blogs,
	published_at	TIMESTAMPTZ,
	title		VARCHAR(1000)	NOT NULL,

	PRIMARY KEY (url, blog)
);
CREATE INDEX ON _r_posts(published_at);
CREATE VIEW posts AS
	SELECT
		p.url, p.blog, p.title, (bind.url IS NOT NULL) is_active,
		p.published_at
	FROM _r_posts p
	INNER JOIN generations g ON g.blog = p.blog
	LEFT JOIN bindings bind ON (bind.gen = g.id AND bind.url = p.url)
	WHERE g.active = true;

CREATE TABLE visits (
	id	SERIAL		PRIMARY KEY,
	url	VARCHAR(1000)	NOT NULL,
	blog	INTEGER		NOT NULL	REFERENCES blogs ON DELETE CASCADE,
	time	TIMESTAMPTZ	NOT NULL	DEFAULT(now())
);
CREATE INDEX ON visits(url);
CREATE INDEX ON visits(blog);
CREATE INDEX ON visits(time);


-- blog subscriber lists

CREATE TYPE subscription_status AS ENUM ('active', 'unsubscribed');

CREATE TABLE subscribers (
	id			SERIAL					PRIMARY KEY,
	blog_id			INTEGER			NOT NULL,
	email			VARCHAR(255)		NOT NULL,
	unsubscribe_token 	UUID			NOT NULL			DEFAULT uuid_generate_v4(),
	status			subscription_status	NOT NULL			DEFAULT('active'),

	created_at		TIMESTAMPTZ		NOT NULL			DEFAULT(now()),

	CONSTRAINT fk_blog_id
		FOREIGN KEY (blog_id)
		REFERENCES blogs(id)
		ON DELETE CASCADE -- delete subscribers when blog deleted
);

--  active subscriber emails should be unique
CREATE UNIQUE INDEX unique_active_subscriber_per_blog
	ON subscribers (email, blog_id) 
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
