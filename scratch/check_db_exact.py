import os
import psycopg2

env_vars = {}
with open('d:/unifai_project/.env', 'r', encoding='utf-8') as f:
    for line in f:
        line = line.strip()
        if line and not line.startswith('#') and '=' in line:
            k, v = line.split('=', 1)
            env_vars[k.strip()] = v.strip().strip('\'"')

conn = psycopg2.connect(
    dbname=env_vars.get('DB_NAME', 'unifai'),
    user=env_vars.get('DB_USER', 'postgres'),
    password=env_vars.get('DB_PASSWORD', 'postgres'),
    host=env_vars.get('DB_HOST', 'localhost'),
    port=env_vars.get('DB_PORT', '5432')
)
cur = conn.cursor()
cur.execute("SELECT created_at, user_prompt_preview, user_prompt_full, metadata FROM browser_ai_logs ORDER BY created_at DESC LIMIT 5;")
for r in cur.fetchall():
    print("TIME:", r[0])
    print("PREVIEW:", repr(r[1]))
    print("FULL:", repr(r[2]))
    print("METADATA:", repr(r[3]))
    print("---")
conn.close()
