import os
import psycopg2
import json

env_vars = {}
with open('d:/unifai_project/.env', 'r', encoding='utf-8') as f:
    for line in f:
        line = line.strip()
        if line and not line.startswith('#') and '=' in line:
            k, v = line.split('=', 1)
            env_vars[k.strip()] = v.strip('\'"')

conn = psycopg2.connect(
    dbname=env_vars.get('DB_NAME', 'unifai'),
    user=env_vars.get('DB_USER', 'postgres'),
    password=env_vars.get('DB_PASSWORD', 'postgres'),
    host=env_vars.get('DB_HOST', 'localhost'),
    port=env_vars.get('DB_PORT', '5432')
)
cur = conn.cursor()

cur.execute("""
    SELECT id, timestamp, user_prompt_preview, user_prompt_full, attachment_name
    FROM browser_ai_logs 
    WHERE attachment_name = 'guard_rules_sample.xlsx'
    ORDER BY timestamp DESC LIMIT 10;
""")
for r in cur.fetchall():
    print(f"ID: {r[0]} | Time: {r[1]} | Preview: {repr(r[2])} | Attach: {repr(r[4])}")

conn.close()
