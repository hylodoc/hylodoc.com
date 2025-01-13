#!/bin/bash

set -x # enables echo

if [ -z "${AWS_ACCESS_KEY_ID}" ] || [ -z "${AWS_SECRET_ACCESS_KEY}" ]; then
	echo "aws keys not set" >&2
	exit 1
fi

if [ -z "${S3_PATH}" ]; then
	echo "s3 path not set" >&2
	exit 2
fi

if 		[ -z "${PGUSER}" ] \
	||	[ -z "${PGPASSWORD}" ] \
	|| 	[ -z "${PGDATABASE}" ] \
	|| 	[ -z "${PGHOST}" ] \
	|| 	[ -z "${PGPORT}" ]; then
	echo "postgres variables not set" >&2
	exit 3
fi

PREFIX='backup'

TEMPDIR=`mktemp -d`
DUMP_FILE=$PREFIX.sql
ZIP_FILE=$PREFIX-`date +%s`.tgz

pg_dump > $TEMPDIR/$DUMP_FILE

tar cfz $TEMPDIR/$ZIP_FILE -C $TEMPDIR $DUMP_FILE

aws s3 cp $TEMPDIR/$ZIP_FILE $S3_PATH/

rm -rf $TEMPDIR
