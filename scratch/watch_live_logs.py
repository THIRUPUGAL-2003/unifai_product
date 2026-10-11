from db_config import get_db_connection
import psycopg2
import time
import sys

conn = get_db_connection()
cur = conn.cursor()

print("=" * 60)
print("  MONITORING LIVE BROWSER AI LOGS IN REAL TIME")
print("  (Type in ChatGPT or Claude to see live intercepts!)")
print("=" * 60, flush=True)

cur.execute("SELECT MAX(created_at) FROM browser_ai_logs")
last_time = cur.fetchone()[0]

count = 0
while count < 30:
    cur.execute(
        "SELECT id, domain, user_prompt_preview, attachment_name, action, rule_triggered, created_at "
        "FROM browser_ai_logs WHERE created_at > %s ORDER BY created_at ASC",
        (last_time,)
    )
    rows = cur.fetchall()
    for r in rows:
        last_time = max(last_time, r[6])
        print(f"\n[NEW EVENT DETECTED!]")
        print(f"  Time:       {r[6].strftime('%H:%M:%S')}")
        print(f"  Domain:     {r[1]}")
        print(f"  Action:     {r[4]}")
        print(f"  Rule:       {r[5]}")
        print(f"  Attachment: {r[3] or 'None'}")
        print(f"  Prompt:     {r[2]}", flush=True)
    time.sleep(2)
    count += 1
