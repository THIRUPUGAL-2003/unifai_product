"""Read plugin rows and login lockout keys. No config secrets."""
import sys
sys.path.insert(0, ".")
from db_config import get_db_connection

conn = get_db_connection()
cur = conn.cursor()
cur.execute("SELECT username_key, failed_count, locked_until FROM auth_login_lockouts ORDER BY id")
print("LOCKOUTS")
for row in cur.fetchall():
    print(row[0], "failed", row[1], "until", row[2])

cur.execute("""
SELECT column_name FROM information_schema.columns
WHERE table_name = 'plugins' OR table_name = 'config_plugins'
ORDER BY table_name, ordinal_position
""")
cols = cur.fetchall()
print("PLUGIN_COLS", [c[0] for c in cols])

cur.execute("""
SELECT table_name FROM information_schema.tables
WHERE table_schema = 'public' AND table_name ILIKE '%plugin%'
""")
print("PLUGIN_TABLES", [r[0] for r in cur.fetchall()])
conn.close()
