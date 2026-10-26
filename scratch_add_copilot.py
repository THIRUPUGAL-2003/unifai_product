import datetime
import os
import uuid
from pathlib import Path

import psycopg2


def _load_project_env() -> None:
    env_path = Path(__file__).resolve().with_name(".env")
    if not env_path.is_file():
        return
    for line in env_path.read_text(encoding="utf-8", errors="ignore").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key, value = key.strip(), value.strip().strip("'\"")
        if key and key not in os.environ:
            os.environ[key] = value


_load_project_env()

_db_type = os.environ.get("DB_TYPE", "").strip().lower()
if _db_type not in {"postgres", "postgresql"}:
    raise SystemExit("scratch_add_copilot.py requires DB_TYPE=postgres in .env")

_required = ("DB_HOST", "DB_NAME", "DB_USER", "DB_PASSWORD")
_missing = [name for name in _required if not os.environ.get(name)]
if _missing:
    raise SystemExit("Missing database settings in .env: " + ", ".join(_missing))

_db_options = {
    "host": os.environ["DB_HOST"],
    "port": int(os.environ.get("DB_PORT") or "5432"),
    "dbname": os.environ["DB_NAME"],
    "user": os.environ["DB_USER"],
    "password": os.environ["DB_PASSWORD"],
}
_sslmode = os.environ.get("DB_SSL_MODE", "").strip()
if _sslmode:
    _db_options["sslmode"] = _sslmode

sql = """
INSERT INTO browser_target_websites (
    id, domain, platform_name, monitored, block_site, intercepted_count,
    status, reply_bot_enabled, reply_bot_provider, reply_bot_model,
    reply_bot_mode, parent_id, created_at, host_role
) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
ON CONFLICT (domain) DO NOTHING
"""

def _new_target(domain: str, parent_id: str = "") -> tuple:
    return (
        "tgt-" + uuid.uuid4().hex,
        domain,
        "Copilot",
        True,
        False,
        0,
        "MONITORED",
        False,
        "",
        "",
        "",
        parent_id,
        datetime.datetime.now(datetime.timezone.utc),
        "ui",
    )


def main() -> None:
    conn = psycopg2.connect(**_db_options)
    try:
        with conn:
            with conn.cursor() as cur:
                cur.execute(
                    "SELECT id, domain FROM browser_target_websites "
                    "WHERE domain IN (%s, %s) FOR UPDATE",
                    ("copilot.com", "www.copilot.com"),
                )
                existing = {domain: target_id for target_id, domain in cur.fetchall()}
                if "copilot.com" not in existing:
                    cur.execute(sql, _new_target("copilot.com"))
                    cur.execute(
                        "SELECT id FROM browser_target_websites WHERE domain = %s",
                        ("copilot.com",),
                    )
                    row = cur.fetchone()
                    if row is None:
                        raise RuntimeError("Failed to create or find the copilot.com root target")
                    existing["copilot.com"] = row[0]

                if "www.copilot.com" not in existing:
                    cur.execute(sql, _new_target("www.copilot.com", existing["copilot.com"]))

                cur.execute(
                    "SELECT domain, platform_name, monitored, status, host_role "
                    "FROM browser_target_websites "
                    "WHERE domain IN (%s, %s) ORDER BY domain",
                    ("copilot.com", "www.copilot.com"),
                )
                for row in cur.fetchall():
                    print(row)
    finally:
        conn.close()


if __name__ == "__main__":
    main()
