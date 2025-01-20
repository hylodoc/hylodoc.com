-- intialise table
DROP DATABASE IF EXISTS knuthic_db;
CREATE DATABASE knuthic_db;
-- connect to db
\c knuthic_db

DROP USER IF EXISTS knuthic_user;
CREATE USER knuthic_user with encrypted password 'secret';

GRANT ALL PRIVILEGES ON DATABASE knuthic_db TO knuthic_user;

SET bytea_output = 'hex';

-- initialise schema
\i /docker-entrypoint-initdb.d/schema/schema.pg.sql
