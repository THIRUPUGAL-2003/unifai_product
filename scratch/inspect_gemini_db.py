import psycopg2
import json

conn = psycopg2.connect(
    dbname="Unifai_test",
    user="Unifai_test",
    password="YP2025-2026yp",
    host="76.13.243.253",
    port=32768
)
cur = conn.cursor()
cur.execute("SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'browser_ai_logs';")
cols = cur.fetchall()
print("Columns in browser_ai_logs:", [c[0] for c in cols])

cur.execute("""
    SELECT id, timestamp, platform, prompt, metadata
    FROM browser_ai_logs
    WHERE platform ILIKE '%gemini%'
    ORDER BY timestamp DESC
    LIMIT 6;
""")
rows = cur.fetchall()
print(f"\nFound {len(rows)} Gemini rows:")
for r in rows:
    print("=" * 60)
    print(f"ID: {r[0]}")
    print(f"Timestamp: {r[1]}")
    print(f"Platform: {r[2]}")
    print(f"Prompt: {r[3]}")
    print(f"Metadata: {json.dumps(r[4], indent=2) if r[4] else None}")
conn.close()
