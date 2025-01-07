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

CREATE TABLE boots (
	id		SERIAL		PRIMARY KEY,
	created_at	TIMESTAMPTZ	NOT NULL	DEFAULT(now())
);
CREATE VIEW boot_id AS
	SELECT id FROM boots ORDER BY id DESC LIMIT 1;


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
	gitdir_path		VARCHAR(1000)	NOT NULL,

	CONSTRAINT fk_gh_installation_id
		FOREIGN KEY (installation_id)
		REFERENCES installations(gh_installation_id)
		ON DELETE CASCADE -- delete repositories when installation deleted
);

CREATE TYPE blog_theme AS ENUM ('lit', 'latex');
CREATE TYPE email_mode AS ENUM ('plaintext', 'html');

CREATE TABLE blogs (
	id 			SERIAL				PRIMARY KEY,
	created_at		TIMESTAMPTZ	NOT NULL			DEFAULT(now()),
	updated_at		TIMESTAMPTZ	NOT NULL			DEFAULT(now()),
	name			VARCHAR(1000),
	user_id			INTEGER		NOT NULL,
	theme			blog_theme	NOT NULL			DEFAULT('lit'),

	subdomain		VARCHAR(255)	NOT NULL
		CONSTRAINT unique_blog_subdomain UNIQUE,
		CHECK (subdomain <> ''),

	domain			VARCHAR(255)			UNIQUE
		CHECK (domain <> ''),

	from_address		VARCHAR(255)	NOT NULL,
	email_mode		email_mode	NOT NULL,
	live_hash		VARCHAR(1000),

	gh_repository_id	BIGINT		NOT NULL	UNIQUE,
	live_branch		VARCHAR(100)	NOT NULL,

	is_live			BOOLEAN		NOT NULL			DEFAULT(false),

	CONSTRAINT fk_user_id
		FOREIGN KEY (user_id)
		REFERENCES users
		ON DELETE CASCADE,

	CONSTRAINT fk_repository_id
		FOREIGN KEY (gh_repository_id)
		REFERENCES repositories(repository_id)
		ON DELETE CASCADE
);

CREATE TABLE reserved_subdomains (
	subdomain		VARCHAR(255)			PRIMARY KEY,
	created_at		TIMESTAMPTZ	NOT NULL			DEFAULT(now())
);

CREATE OR REPLACE FUNCTION check_reserved_subdomain()
RETURNS TRIGGER AS $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM reserved_subdomains
		WHERE subdomain = NEW.subdomain
	) THEN RAISE EXCEPTION '"%" is reserved', NEW.subdomain;
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION check_used_subdomain()
RETURNS TRIGGER AS $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM blogs
		WHERE subdomain = NEW.subdomain
	) THEN RAISE EXCEPTION '"%" is used', NEW.subdomain;
	END IF;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER check_reserved_subdomain_trigger
BEFORE INSERT OR UPDATE ON blogs
FOR EACH ROW
EXECUTE FUNCTION check_reserved_subdomain();

CREATE TRIGGER check_used_subdomain_trigger
BEFORE INSERT OR UPDATE ON reserved_subdomains
FOR EACH ROW
EXECUTE FUNCTION check_used_subdomain();

CREATE TABLE generations (
	id		SERIAL		PRIMARY KEY,
	created_at	TIMESTAMPTZ	NOT NULL	DEFAULT(now()),
	hash		VARCHAR(1000)	NOT NULL,
	boot_id		INTEGER		NOT NULL	REFERENCES boots,
	stale		BOOLEAN		NOT NULL	DEFAULT(false)
);
CREATE INDEX ON generations(stale);
CREATE INDEX ON generations(boot_id);
CREATE UNIQUE INDEX unique_hash_boot_id
	ON generations (hash, boot_id)
	WHERE stale = false;

CREATE TABLE bindings (
	gen 	INTEGER		NOT NULL	REFERENCES generations,
	url 	VARCHAR(1000)	NOT NULL,
	path	VARCHAR(1000)	NOT NULL,

	PRIMARY KEY (gen, url)
);

CREATE TABLE post_email_bindings (
	gen		INTEGER		NOT NULL,
	url		VARCHAR(1000)	NOT NULL, 	PRIMARY KEY (gen, url),
							FOREIGN KEY (gen, url)
							REFERENCES bindings(gen, url),
	html		VARCHAR(1000)	NOT NULL,
	text		VARCHAR(1000)	NOT NULL
);

CREATE TABLE _r_posts (
	url		VARCHAR(1000)	NOT NULL,
	blog		INTEGER		NOT NULL	REFERENCES blogs,
	published_at	TIMESTAMPTZ,
	title		VARCHAR(1000)	NOT NULL,

	email_token	UUID		NOT NULL	UNIQUE	DEFAULT uuid_generate_v4(),
	email_sent	BOOLEAN		NOT NULL	DEFAULT(false),

	PRIMARY KEY (url, blog)
);
CREATE INDEX ON _r_posts(published_at);
CREATE VIEW posts AS
	SELECT
		p.url,
		p.blog,
		p.title,
		(bind.url IS NOT NULL)::BOOLEAN is_active,
		p.published_at,
		p.email_token,
		p.email_sent,
		peb.html html_email_path,
		peb.text text_email_path
	FROM _r_posts p
	INNER JOIN blogs b ON b.id = p.blog
	INNER JOIN generations g ON g.hash = b.live_hash
	INNER JOIN boot_id on boot_id.id = g.boot_id
	LEFT JOIN (
		bindings bind
		INNER JOIN post_email_bindings peb
			ON (peb.gen = bind.gen AND peb.url = bind.url)
	) ON (bind.gen = g.id AND bind.url = p.url)
	WHERE g.stale = false;

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
	unsubscribe_token 	UUID			NOT NULL	UNIQUE		DEFAULT uuid_generate_v4(),
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

CREATE TABLE subscriber_emails (
	token		UUID		PRIMARY KEY	DEFAULT uuid_generate_v4(),
	subscriber	INTEGER		NOT NULL	REFERENCES subscribers,

	url		VARCHAR(1000)	NOT NULL,
	blog		INTEGER		NOT NULL, 	FOREIGN KEY (url, blog)
							REFERENCES _r_posts (url, blog),
	clicked 	BOOLEAN		NOT NULL	DEFAULT(false),

	UNIQUE (subscriber, url, blog)
);
CREATE INDEX ON subscriber_emails(subscriber);
CREATE INDEX ON subscriber_emails(clicked);
CREATE INDEX ON subscriber_emails(url, blog);


-- NB: these must match the names on Stripe
CREATE TYPE sub_name AS ENUM ('Basic', 'Premium');

CREATE TABLE stripe_subscriptions (
	stripe_subscription_id	VARCHAR(255)		NOT NULL	PRIMARY KEY,
	stripe_customer_id	VARCHAR(255)		NOT NULL,
	stripe_status		VARCHAR(255)		NOT NULL,
	sub_name		sub_name		NOT NULL,

	user_id			INTEGER			NOT NULL	REFERENCES users,
	created_at		TIMESTAMPTZ		NOT NULL	DEFAULT(now()),
	updated_at		TIMESTAMPTZ		NOT NULL	DEFAULT(now())
);

CREATE TYPE queued_email_status AS ENUM ('pending', 'sent', 'failed');
CREATE TYPE postmark_stream AS ENUM ('broadcast', 'outbound');

CREATE TABLE queued_emails (
	id		SERIAL			PRIMARY KEY,
	created_at	TIMESTAMPTZ		NOT NULL	DEFAULT(now()),
	status		queued_email_status	NOT NULL	DEFAULT('pending'),
	fail_count	INTEGER			NOT NULL	DEFAULT(0),

	from_addr	VARCHAR(1000)		NOT NULL,
	to_addr		VARCHAR(1000)		NOT NULL,
	subject		VARCHAR(1000)		NOT NULL,
	body		TEXT			NOT NULL,
	mode		email_mode		NOT NULL,
	stream		postmark_stream		NOT NULL,

	ended_at	TIMESTAMPTZ
		CHECK (status = 'pending' OR ended_at IS NOT NULL)
);
CREATE INDEX ON queued_emails(created_at);
CREATE INDEX ON queued_emails(status);
CREATE INDEX ON queued_emails(fail_count);

CREATE TABLE queued_email_headers (
	email	INTEGER		NOT NULL	REFERENCES queued_emails,
	name	VARCHAR(1000)	NOT NULL,
	value	VARCHAR(1000)	NOT NULL,

	PRIMARY KEY (email, name)
);

CREATE TABLE queued_email_postmark_error (
	id		SERIAL		PRIMARY KEY,
	timestamp	TIMESTAMPTZ 	NOT NULL	DEFAULT(now()),
	email		INTEGER		NOT NULL	REFERENCES queued_emails,
	code		INTEGER		NOT NULL,
	message		TEXT		NOT NULL
);
CREATE INDEX ON queued_email_postmark_error(email);
