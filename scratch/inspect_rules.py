from db_config import get_db_connection
import psycopg2

conn = get_db_connection()
cur = conn.cursor()
cur.execute("SELECT id, name, rule_type, pattern, action, active FROM browser_guard_rules WHERE active=true")
rows = cur.fetchall()
print('Active rules count:', len(rows))
for r in rows:
    print(' ', r)
