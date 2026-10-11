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

# Check browser_target_websites
cur.execute("SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'browser_target_websites' ORDER BY ordinal_position;")
cols = cur.fetchall()
print("browser_target_websites columns:", cols)

cur.execute("SELECT count(*) FROM browser_target_websites;")
print("browser_target_websites count:", cur.fetchone()[0])

cur.execute("SELECT domain, platform_name, host_role, monitored, block_site FROM browser_target_websites LIMIT 5;")
for r in cur.fetchall():
    print(" ", r)

# Check browser_guard_rules
cur.execute("SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'browser_guard_rules' ORDER BY ordinal_position;")
cols_rules = cur.fetchall()
print("\nbrowser_guard_rules columns:", cols_rules)

cur.execute("SELECT count(*) FROM browser_guard_rules;")
print("browser_guard_rules count:", cur.fetchone()[0])

cur.execute("SELECT id, name, pattern, action, active, rule_type FROM browser_guard_rules LIMIT 10;")
for r in cur.fetchall():
    print(" ", r)

conn.close()
