import psycopg2

conn = psycopg2.connect(
    host='76.13.243.253',
    port=32768,
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    connect_timeout=5
)
cur = conn.cursor()
cur.execute("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name;")
tables = [r[0] for r in cur.fetchall()]
print("Matching tables:", [t for t in tables if 'browser' in t or 'log' in t or 'prompt' in t])

cur.execute("SELECT * FROM browser_ai_logs WHERE id = '3186aadc-87e1-465e-8deb-edc0de1eaf80';")
row = cur.fetchone()
cur.execute("SELECT column_name FROM information_schema.columns WHERE table_name = 'browser_ai_logs';")
cols = [c[0] for c in cur.fetchall()]
for col, val in zip(cols, row):
    print(f"{col}: {val}")
conn.close()
