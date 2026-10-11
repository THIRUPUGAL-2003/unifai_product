import psycopg2

conn = psycopg2.connect(
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    host='76.13.243.253',
    port=32768
)
cur = conn.cursor()
cur.execute("""
SELECT id, created_at, user_prompt_preview, attachment_name, action
FROM browser_ai_logs
WHERE attachment_name ILIKE '%Test_Document_Audit%' OR user_prompt_preview ILIKE '%Test_Document_Audit%'
ORDER BY created_at DESC;
""")
rows = cur.fetchall()
print('Found rows:', len(rows))
for r in rows:
    print(' ', r)
