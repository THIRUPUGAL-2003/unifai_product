import os
import sys
from pathlib import Path
import psycopg2
import json

def get_db_conn():
    env_vars = {}
    for p in [Path(__file__).resolve().parents[5] / ".env", Path.cwd() / ".env"]:
        if p.is_file():
            with open(p, "r", encoding="utf-8", errors="ignore") as f:
                for line in f:
                    line = line.strip()
                    if line and not line.startswith("#") and "=" in line:
                        k, v = line.split("=", 1)
                        env_vars[k.strip()] = v.strip().strip("'\"")
            break
    host = env_vars.get("DB_HOST") or os.environ.get("DB_HOST", "127.0.0.1")
    port = int(env_vars.get("DB_PORT") or os.environ.get("DB_PORT", "5432"))
    dbname = env_vars.get("DB_NAME") or os.environ.get("DB_NAME", "")
    user = env_vars.get("DB_USER") or os.environ.get("DB_USER", "")
    password = env_vars.get("DB_PASSWORD") or os.environ.get("DB_PASSWORD", "")
    return psycopg2.connect(host=host, port=port, dbname=dbname, user=user, password=password)

conn = get_db_conn()
cur = conn.cursor()
cur.execute("SELECT id, attachment_name, attachment_size_bytes, status, action, rule_triggered, metadata FROM browser_ai_logs WHERE attachment_name ILIKE '%docx%' ORDER BY id DESC LIMIT 5;")
for r in cur.fetchall():
    print("="*60)
    print("ID:", r[0])
    print("Name:", r[1])
    print("Size:", r[2])
    print("Status:", r[3])
    print("Action:", r[4])
    print("Rule:", r[5])
    print("Metadata:", json.dumps(r[6], indent=2) if isinstance(r[6], dict) else r[6])
