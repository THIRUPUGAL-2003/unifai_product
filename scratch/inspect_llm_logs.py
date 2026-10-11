from db_config import get_db_connection
import psycopg2

conn = get_db_connection()
cur = conn.cursor()

cur.execute("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name LIKE '%log%';")
tables = [r[0] for r in cur.fetchall()]
print('Log tables found:', tables)

# Check llm request logs / request_logs / requests
cur.execute("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND (table_name LIKE '%req%' OR table_name LIKE '%prompt%');")
print('Req/prompt tables:', [r[0] for r in cur.fetchall()])

cur.close()
conn.close()
