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
    SELECT id, attachment_content_type, metadata
    FROM browser_ai_logs 
    WHERE id = 'bb671eef-73fa-46fd-aec7-3b54443a1cd2';
""")
r = cur.fetchone()
print("ID:", r[0])
print("Content-Type:", repr(r[1]))
print("Meta:", repr(r[2]))

conn.close()
