import os
from pathlib import Path

import psycopg2
import uuid
import datetime


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

conn = psycopg2.connect(**_db_options)
cur = conn.cursor()
cur.execute("SELECT domain FROM browser_target_websites WHERE domain IN ('copilot.com', 'www.copilot.com')")
existing = [r[0] for r in cur.fetchall()]
print('Existing:', existing)

sql = """
INSERT INTO browser_target_websites (
    id, domain, platform_name, monitored, block_site, intercepted_count,
    status, reply_bot_enabled, reply_bot_provider, reply_bot_model,
    reply_bot_mode, parent_id, created_at, host_role
) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
"""

if 'copilot.com' not in existing:
    cur.execute(sql, (
        'tgt-' + uuid.uuid4().hex[:8], 'copilot.com', 'Copilot', True, False, 0,
        'MONITORED', False, '', '', '', 'tgt-df3d0ba5', datetime.datetime.now(datetime.timezone.utc), 'ui'
    ))
    print('Inserted copilot.com')

if 'www.copilot.com' not in existing:
    cur.execute(sql, (
        'tgt-' + uuid.uuid4().hex[:8], 'www.copilot.com', 'Copilot', True, False, 0,
        'MONITORED', False, '', '', '', 'tgt-df3d0ba5', datetime.datetime.now(datetime.timezone.utc), 'ui'
    ))
    print('Inserted www.copilot.com')

conn.commit()
cur.execute("SELECT domain, platform_name, monitored, status, host_role FROM browser_target_websites WHERE platform_name='Copilot' ORDER BY domain")
for r in cur.fetchall():
    print(r)
