from db_config import get_db_connection
import psycopg2

conn = get_db_connection()
cur = conn.cursor()

# Get column names
cur.execute("SELECT column_name FROM information_schema.columns WHERE table_name = 'browser_ai_agents';")
cols = [r[0] for r in cur.fetchall()]
print("Columns in browser_ai_agents:", cols)

cur.execute("SELECT * FROM browser_ai_agents ORDER BY 1 DESC LIMIT 5;")
rows = cur.fetchall()
print(f"Total rows retrieved: {len(rows)}")
for row in rows:
    row_dict = dict(zip(cols, row))
    print({k: str(v) for k, v in row_dict.items() if v is not None})

cur.close()
conn.close()
