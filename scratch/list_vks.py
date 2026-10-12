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
cur.execute("SELECT id, name, value, is_active FROM governance_virtual_keys ORDER BY created_at DESC")
rows = cur.fetchall()
print(f"Total Virtual Keys in DB: {len(rows)}")
for r in rows:
    print(f"ID: {r[0]}, Name: {r[1]}, Value: {r[2][:15]}..., Active: {r[3]}")
conn.close()
