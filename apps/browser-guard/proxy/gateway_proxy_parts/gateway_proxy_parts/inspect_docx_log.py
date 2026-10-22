import psycopg2
import json

conn = psycopg2.connect('postgresql://Unifai_test:YP2025-2026yp@76.13.243.253:32768/Unifai_test')
cur = conn.cursor()
cur.execute("SELECT id, attachment_name, attachment_size_bytes, status, action, rule_triggered, metadata FROM browser_ai_logs WHERE attachment_name ILIKE '%docx%' ORDER BY id DESC LIMIT 5;")
for r in cur.fetchall():
    print("="*60)
    print("ID:", r[0])
    print("Name:", r[1])
    print("Size:", r[2])
    print("Status:", r[3])
    print("Action:", r[4])
    print("Rule:", r[5])
    print("Metadata:", json.dumps(r[6], indent=2) if isinstance(r[6], dict) else r[6])
