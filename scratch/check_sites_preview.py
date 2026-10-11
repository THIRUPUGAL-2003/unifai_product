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
cur.execute("SELECT id, created_at, user_prompt_preview, attachment_name, metadata FROM browser_ai_logs WHERE user_prompt_preview LIKE '%sites-preview%' OR attachment_name LIKE '%sites-preview%' ORDER BY created_at DESC LIMIT 10;")
rows = cur.fetchall()
for r in rows:
    print(r[1], "|", r[2], "| attachment_name:", r[3])
conn.close()
