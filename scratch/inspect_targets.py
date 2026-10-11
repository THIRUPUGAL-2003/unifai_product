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
cur.execute("SELECT COUNT(*), platform_name FROM browser_target_websites GROUP BY platform_name ORDER BY count DESC;")
for row in cur.fetchall():
    print(row)
cur.execute("SELECT COUNT(*) FROM browser_target_websites;")
print("Total target websites:", cur.fetchone()[0])
conn.close()
