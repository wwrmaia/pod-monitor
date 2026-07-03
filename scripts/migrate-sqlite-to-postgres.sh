#!/usr/bin/env bash
# migrate-sqlite-to-postgres.sh
#
# Migrates pod-monitor data from a SQLite database to PostgreSQL.
#
# Requirements:
#   - sqlite3 CLI
#   - psql CLI
#   - PostgreSQL instance reachable from this machine
#
# Usage:
#   ./migrate-sqlite-to-postgres.sh \
#     --sqlite /path/to/pod-monitor.db \
#     --pg-url "postgres://podmonitor:changeme@localhost:5432/podmonitor?sslmode=disable"
#
# The script is idempotent for most tables (INSERT OR IGNORE / ON CONFLICT DO NOTHING).
# pod_snapshots is an exception: it appends without deduplication (no natural unique key).
# Run once against a stopped or read-only pod-monitor instance to avoid partial imports.

set -euo pipefail

# ── Argument parsing ──────────────────────────────────────────────────────────

SQLITE_PATH=""
PG_URL=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --sqlite)    SQLITE_PATH="$2"; shift 2 ;;
    --pg-url)    PG_URL="$2";      shift 2 ;;
    *)           echo "Unknown argument: $1"; exit 1 ;;
  esac
done

if [[ -z "$SQLITE_PATH" || -z "$PG_URL" ]]; then
  echo "Usage: $0 --sqlite <path> --pg-url <postgres-dsn>"
  exit 1
fi

if [[ ! -f "$SQLITE_PATH" ]]; then
  echo "SQLite file not found: $SQLITE_PATH"
  exit 1
fi

command -v sqlite3 >/dev/null 2>&1 || { echo "sqlite3 not found in PATH"; exit 1; }
command -v psql    >/dev/null 2>&1 || { echo "psql not found in PATH";    exit 1; }

echo "Source : $SQLITE_PATH"
echo "Target : $PG_URL"
echo

# ── Helper: run SQL against PostgreSQL ───────────────────────────────────────

pg() { psql "$PG_URL" --quiet --no-align --tuples-only -c "$1"; }
pg_file() { psql "$PG_URL" --quiet -f "$1"; }

# ── Helper: SQLite table row count ───────────────────────────────────────────

sqlite_count() { sqlite3 "$SQLITE_PATH" "SELECT COUNT(*) FROM $1 ;" 2>/dev/null || echo 0; }

# ── Verify connection ─────────────────────────────────────────────────────────

echo "Testing PostgreSQL connection..."
psql "$PG_URL" --quiet -c "SELECT 1" >/dev/null
echo "Connected."
echo

# ── 1. users ──────────────────────────────────────────────────────────────────

TABLE=users
COUNT=$(sqlite_count $TABLE)
echo "Migrating $TABLE ($COUNT rows)..."

sqlite3 "$SQLITE_PATH" "SELECT username, password, role, allowed_clusters, allowed_namespaces, totp_secret, totp_enabled, group_name FROM users ;" \
| while IFS='|' read -r username password role allowed_clusters allowed_namespaces totp_secret totp_enabled group_name; do
  psql "$PG_URL" --quiet -c \
    "INSERT INTO users (username, password, role, allowed_clusters, allowed_namespaces, totp_secret, totp_enabled, group_name)
     VALUES ($(printf '%s' "$username" | sed "s/'/''/g" | xargs -I{} echo "'{}' "), \
             $(printf '%s' "$password" | sed "s/'/''/g" | xargs -I{} echo "'{}' "), \
             $(printf '%s' "$role" | sed "s/'/''/g" | xargs -I{} echo "'{}' "), \
             $(printf '%s' "$allowed_clusters" | sed "s/'/''/g" | xargs -I{} echo "'{}' "), \
             $(printf '%s' "$allowed_namespaces" | sed "s/'/''/g" | xargs -I{} echo "'{}' "), \
             $(printf '%s' "$totp_secret" | sed "s/'/''/g" | xargs -I{} echo "'{}' "), \
             $totp_enabled, \
             $(printf '%s' "$group_name" | sed "s/'/''/g" | xargs -I{} echo "'{}' ") \
     )
     ON CONFLICT (username) DO NOTHING;" 2>/dev/null || true
done 2>/dev/null || true

# Use CSV export for safer quoting
TMP=$(mktemp /tmp/pm-migrate-XXXXXX.csv)
trap 'rm -f "$TMP"' EXIT

echo "Migrating $TABLE via COPY..."
sqlite3 -separator $'\t' "$SQLITE_PATH" \
  "SELECT username, password, role, COALESCE(allowed_clusters,''), COALESCE(allowed_namespaces,''), COALESCE(totp_secret,''), COALESCE(totp_enabled,0), COALESCE(group_name,'') FROM users ;" \
  > "$TMP"

psql "$PG_URL" --quiet -c \
  "\COPY users(username,password,role,allowed_clusters,allowed_namespaces,totp_secret,totp_enabled,group_name) FROM '$TMP' WITH (FORMAT text, DELIMITER E'\t', NULL '')" 2>/dev/null || \
  echo "  [warn] COPY for $TABLE failed (rows may already exist — this is OK if re-running)"

rm -f "$TMP"
echo "  done."

# ── 2. groups ─────────────────────────────────────────────────────────────────

TABLE=groups
COUNT=$(sqlite_count $TABLE)
echo "Migrating $TABLE ($COUNT rows)..."

TMP=$(mktemp /tmp/pm-migrate-XXXXXX.tsv)
sqlite3 -separator $'\t' "$SQLITE_PATH" \
  "SELECT name, role, COALESCE(totp_enabled,0), COALESCE(allowed_clusters,''), COALESCE(allowed_namespaces,'') FROM groups ;" \
  > "$TMP"

psql "$PG_URL" --quiet -c \
  "\COPY groups(name,role,totp_enabled,allowed_clusters,allowed_namespaces) FROM '$TMP' WITH (FORMAT text, DELIMITER E'\t', NULL '')" 2>/dev/null || \
  echo "  [warn] COPY for $TABLE skipped (rows may already exist)"

rm -f "$TMP"
echo "  done."

# ── 3. pod_snapshots ──────────────────────────────────────────────────────────

TABLE=pod_snapshots
COUNT=$(sqlite_count $TABLE)
echo "Migrating $TABLE ($COUNT rows) — this may take a while..."

TMP=$(mktemp /tmp/pm-migrate-XXXXXX.tsv)
sqlite3 -separator $'\t' "$SQLITE_PATH" \
  "SELECT session_id, COALESCE(captured_at,''), COALESCE(cluster,''), COALESCE(namespace,''), COALESCE(pod,''), COALESCE(node,''), COALESCE(container,''), COALESCE(cpu_request,''), COALESCE(cpu_limit,''), COALESCE(mem_request,''), COALESCE(mem_limit,''), COALESCE(cpu_usage,''), COALESCE(mem_usage,'') FROM pod_snapshots ORDER BY id ;" \
  > "$TMP"

psql "$PG_URL" --quiet -c \
  "\COPY pod_snapshots(session_id,captured_at,cluster,namespace,pod,node,container,cpu_request,cpu_limit,mem_request,mem_limit,cpu_usage,mem_usage) FROM '$TMP' WITH (FORMAT text, DELIMITER E'\t', NULL '')"

rm -f "$TMP"
echo "  done."

# ── 4. dashboards ─────────────────────────────────────────────────────────────

TABLE=dashboards
COUNT=$(sqlite_count $TABLE)
echo "Migrating $TABLE ($COUNT rows)..."

TMP=$(mktemp /tmp/pm-migrate-XXXXXX.tsv)
sqlite3 -separator $'\t' "$SQLITE_PATH" \
  "SELECT username, COALESCE(name,'Meu Dashboard'), COALESCE(widgets,'[]') FROM dashboards ;" \
  > "$TMP"

psql "$PG_URL" --quiet -c \
  "\COPY dashboards(username,name,widgets) FROM '$TMP' WITH (FORMAT text, DELIMITER E'\t', NULL '')" 2>/dev/null || \
  echo "  [warn] COPY for $TABLE skipped"

rm -f "$TMP"
echo "  done."

# ── 5. alert_thresholds ───────────────────────────────────────────────────────

TABLE=alert_thresholds
COUNT=$(sqlite_count $TABLE)
echo "Migrating $TABLE ($COUNT rows)..."

TMP=$(mktemp /tmp/pm-migrate-XXXXXX.tsv)
sqlite3 -separator $'\t' "$SQLITE_PATH" \
  "SELECT COALESCE(cluster,''), COALESCE(namespace,''), COALESCE(warn_pct,85), COALESCE(crit_pct,90) FROM alert_thresholds ;" \
  > "$TMP"

# Use ON CONFLICT so re-runs are idempotent
while IFS=$'\t' read -r cluster namespace warn_pct crit_pct; do
  psql "$PG_URL" --quiet -c \
    "INSERT INTO alert_thresholds(cluster,namespace,warn_pct,crit_pct)
     VALUES ('$(echo "$cluster" | sed "s/'/''/g")', '$(echo "$namespace" | sed "s/'/''/g")', $warn_pct, $crit_pct)
     ON CONFLICT (cluster,namespace) DO UPDATE SET warn_pct=EXCLUDED.warn_pct, crit_pct=EXCLUDED.crit_pct;" 2>/dev/null || true
done < "$TMP"

rm -f "$TMP"
echo "  done."

# ── 6. audit_log ──────────────────────────────────────────────────────────────

TABLE=audit_log
COUNT=$(sqlite_count $TABLE)
echo "Migrating $TABLE ($COUNT rows)..."

TMP=$(mktemp /tmp/pm-migrate-XXXXXX.tsv)
sqlite3 -separator $'\t' "$SQLITE_PATH" \
  "SELECT COALESCE(timestamp,''), COALESCE(username,''), COALESCE(role,''), COALESCE(action,''), COALESCE(detail,''), COALESCE(ip,'') FROM audit_log ORDER BY id ;" \
  > "$TMP"

psql "$PG_URL" --quiet -c \
  "\COPY audit_log(timestamp,username,role,action,detail,ip) FROM '$TMP' WITH (FORMAT text, DELIMITER E'\t', NULL '')" 2>/dev/null || \
  echo "  [warn] COPY for $TABLE skipped"

rm -f "$TMP"
echo "  done."

# ── 7. webhooks ───────────────────────────────────────────────────────────────

TABLE=webhooks
COUNT=$(sqlite_count $TABLE)
echo "Migrating $TABLE ($COUNT rows)..."

TMP=$(mktemp /tmp/pm-migrate-XXXXXX.tsv)
sqlite3 -separator $'\t' "$SQLITE_PATH" \
  "SELECT COALESCE(name,''), COALESCE(url,''), COALESCE(events,'critical'), COALESCE(enabled,1) FROM webhooks ;" \
  > "$TMP"

psql "$PG_URL" --quiet -c \
  "\COPY webhooks(name,url,events,enabled) FROM '$TMP' WITH (FORMAT text, DELIMITER E'\t', NULL '')" 2>/dev/null || \
  echo "  [warn] COPY for $TABLE skipped"

rm -f "$TMP"
echo "  done."

# ── Summary ───────────────────────────────────────────────────────────────────

echo
echo "Migration complete. Row counts in PostgreSQL:"
for t in users groups pod_snapshots dashboards alert_thresholds audit_log webhooks; do
  n=$(psql "$PG_URL" --quiet --no-align --tuples-only -c "SELECT COUNT(*) FROM $t;")
  printf "  %-20s %s rows\n" "$t" "$n"
done
echo
echo "Next steps:"
echo "  1. Start the backend with DATABASE_URL pointing to this PostgreSQL instance."
echo "  2. Verify the UI shows expected data."
echo "  3. Remove the old SQLite file and DB_PATH environment variable."
