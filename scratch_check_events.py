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
cur.execute("""
    SELECT id, user_prompt, platform, action, raw_request_preview, created_at 
    FROM browser_ai_events 
    ORDER BY created_at DESC 
    LIMIT 20;
""")
for r in cur.fetchall():
    print(r[0], "| prompt:", repr(r[1]), "| action:", r[3], "| time:", r[5])
    print("  preview:", repr(r[4])[:200])
conn.close()
