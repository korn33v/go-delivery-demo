#!/bin/sh
set -eu
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
 -v order_password="$ORDER_DB_PASSWORD" -v payment_password="$PAYMENT_DB_PASSWORD" -v fulfillment_password="$FULFILLMENT_DB_PASSWORD" <<'SQL'
CREATE USER order_user PASSWORD :'order_password';
CREATE USER payment_user PASSWORD :'payment_password';
CREATE USER fulfillment_user PASSWORD :'fulfillment_password';
CREATE DATABASE order_db OWNER order_user;
CREATE DATABASE payment_db OWNER payment_user;
CREATE DATABASE fulfillment_db OWNER fulfillment_user;
REVOKE CONNECT ON DATABASE order_db FROM PUBLIC;
REVOKE CONNECT ON DATABASE payment_db FROM PUBLIC;
REVOKE CONNECT ON DATABASE fulfillment_db FROM PUBLIC;
GRANT CONNECT ON DATABASE order_db TO order_user;
GRANT CONNECT ON DATABASE payment_db TO payment_user;
GRANT CONNECT ON DATABASE fulfillment_db TO fulfillment_user;
SQL
