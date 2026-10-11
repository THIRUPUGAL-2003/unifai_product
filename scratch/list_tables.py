from db_config import get_db_connection
import psycopg2

conn = get_db_connection()
cur = conn.cursor()

cur.execute("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name;")
all_tables = [r[0] for r in cur.fetchall()]
print("All tables in DB:", all_tables)

cur.close()
conn.close()
