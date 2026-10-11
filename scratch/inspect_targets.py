import psycopg2

conn = psycopg2.connect(
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    host='76.13.243.253',
    port=32768
)
cur = conn.cursor()
cur.execute("SELECT column_name FROM information_schema.columns WHERE table_name='browser_target_websites'")
print('Columns:', [r[0] for r in cur.fetchall()])
cur.execute("SELECT * FROM browser_target_websites")
for r in cur.fetchall():
    print(r)
