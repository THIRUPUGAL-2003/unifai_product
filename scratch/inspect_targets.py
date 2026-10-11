from db_config import get_db_connection
import psycopg2

conn = get_db_connection()
cur = conn.cursor()
cur.execute("SELECT column_name FROM information_schema.columns WHERE table_name='browser_target_websites'")
print('Columns:', [r[0] for r in cur.fetchall()])
cur.execute("SELECT * FROM browser_target_websites")
for r in cur.fetchall():
    print(r)
