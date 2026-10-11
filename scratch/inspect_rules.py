import psycopg2

conn = psycopg2.connect(
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    host='76.13.243.253',
    port=32768
)
cur = conn.cursor()
cur.execute("SELECT id, name, rule_type, pattern, action, active FROM browser_guard_rules WHERE active=true")
rows = cur.fetchall()
print('Active rules count:', len(rows))
for r in rows:
    print(' ', r)
