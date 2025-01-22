-- intialise table
DROP DATABASE IF EXISTS hylodoc_db;
CREATE DATABASE hylodoc_db;
-- connect to db
\c hylodoc_db

DROP USER IF EXISTS hylodoc_user;
CREATE USER hylodoc_user with encrypted password 'secret';

GRANT ALL PRIVILEGES ON DATABASE hylodoc_db TO hylodoc_user;

SET bytea_output = 'hex';

-- initialise schema
\i /docker-entrypoint-initdb.d/schema/schema.pg.sql
