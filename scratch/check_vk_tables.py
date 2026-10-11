from db_config import get_db_connection
import psycopg2

conn = get_db_connection()
cur = conn.cursor()

# Find tables relating to virtual keys
cur.execute("""
    SELECT table_name FROM information_schema.tables 
    WHERE table_schema = 'public' AND table_name LIKE '%virtual_key%';
""")
print('Virtual key tables:', [r[0] for r in cur.fetchall()])

cur.close()
conn.close()
