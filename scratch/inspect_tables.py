import psycopg2

conn = psycopg2.connect(
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    host='76.13.243.253',
    port=32768
)
cur = conn.cursor()
cur.execute("SELECT table_name FROM information_schema.tables WHERE table_schema='public'")
rows = cur.fetchall()
tables = [r[0] for r in rows if any(k in r[0].lower() for k in ['guard', 'ai', 'rule', 'target', 'log'])]
print('Tables:', tables)
