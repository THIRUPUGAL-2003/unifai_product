import psycopg2

conn = psycopg2.connect('postgresql://Unifai_test:YP2025-2026yp@76.13.243.253:32768/Unifai_test')
cur = conn.cursor()
cur.execute("SELECT column_name FROM information_schema.columns WHERE table_name = 'circuit_breaker_policies';")
cols = [r[0] for r in cur.fetchall()]
print("Columns in circuit_breaker_policies:", cols)

cur.execute("SELECT * FROM circuit_breaker_policies LIMIT 5;")
rows = cur.fetchall()
print(f"Total Circuit Breaker Policies in DB: {len(rows)}")
for r in rows:
    print(dict(zip(cols, r)))

cur.close()
conn.close()
