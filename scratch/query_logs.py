from db_config import get_db_connection
import psycopg2

conn = get_db_connection()
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
