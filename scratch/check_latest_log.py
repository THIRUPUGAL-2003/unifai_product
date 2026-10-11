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
cur.execute("SELECT column_name FROM information_schema.columns WHERE table_name = 'browser_ai_logs' ORDER BY ordinal_position;")
cols = [r[0] for r in cur.fetchall()]
print("Columns:", cols)

cur.execute("SELECT * FROM browser_ai_logs ORDER BY created_at DESC LIMIT 3;")
rows = cur.fetchall()
for row in rows:
    print("--- LOG ENTRY ---")
    for col, val in zip(cols, row):
        if col in ("user_prompt", "full_prompt", "file_name", "action", "platform", "domain", "created_at", "extracted_text", "metadata", "scan_guard"):
            print(f"  {col}: {val}")
conn.close()
