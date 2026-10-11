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
cur.execute("SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'browser_target_websites' ORDER BY ordinal_position;")
for r in cur.fetchall():
    print(r)
print("--- Sample rows ---")
cur.execute("SELECT domain, platform_name, host_role, monitored, block_entire_website, status FROM browser_target_websites LIMIT 10;")
for r in cur.fetchall():
    print(r)
conn.close()
