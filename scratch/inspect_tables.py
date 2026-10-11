from db_config import get_db_connection
import psycopg2

conn = get_db_connection()
cur = conn.cursor()
cur.execute("SELECT table_name FROM information_schema.tables WHERE table_schema='public'")
rows = cur.fetchall()
tables = [r[0] for r in rows if any(k in r[0].lower() for k in ['guard', 'ai', 'rule', 'target', 'log'])]
print('Tables:', tables)
