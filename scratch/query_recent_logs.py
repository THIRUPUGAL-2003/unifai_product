import psycopg2

conn = psycopg2.connect(
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    host='76.13.243.253',
    port=32768
)
cur = conn.cursor()
cur.execute("SELECT id, created_at, domain, user_prompt_preview, attachment_name, action, rule_triggered FROM browser_ai_logs ORDER BY created_at DESC LIMIT 5")
for r in cur.fetchall():
    print(r)
