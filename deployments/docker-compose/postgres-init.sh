#!/bin/sh
set -eu

psql --set=ON_ERROR_STOP=1 \
  --set=app_user="$APP_POSTGRES_USER" \
  --set=app_password="$APP_POSTGRES_PASSWORD" \
  --set=app_database="$POSTGRES_DB" \
  --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<'SQL'
CREATE ROLE :"app_user" LOGIN PASSWORD :'app_password'
  NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
ALTER DATABASE :"app_database" OWNER TO :"app_user";
ALTER ROLE CURRENT_USER PASSWORD NULL;
SQL
