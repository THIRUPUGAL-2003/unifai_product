from db_config import get_db_connection
import psycopg2

conn = get_db_connection()
cur = conn.cursor()
cur.execute("SELECT id, created_at, domain, user_prompt_preview, attachment_name, action, rule_triggered FROM browser_ai_logs ORDER BY created_at DESC LIMIT 5")
for r in cur.fetchall():
    print(r)
