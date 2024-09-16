-- intialise table
DROP DATABASE IF EXISTS progstack_db;
CREATE DATABASE progstack_db;
-- connect to db
\c progstack_db

DROP USER IF EXISTS progstack_user;
CREATE USER progstack_user with encrypted password 'secret';

GRANT ALL PRIVILEGES ON DATABASE progstack_db TO progstack_user;

SET bytea_output = 'hex';

-- initialise schema
\i /docker-entrypoint-initdb.d/schema/schema.pg.sql
