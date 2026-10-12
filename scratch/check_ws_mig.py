import psycopg2

conn = psycopg2.connect(
    host='76.13.243.253',
    port=32768,
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    sslmode='disable'
)
cur = conn.cursor()
cur.execute("SELECT id FROM migrations WHERE id LIKE '%workspace%'")
print(cur.fetchall())
conn.close()
