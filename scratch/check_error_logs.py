import psycopg2
import json

conn = psycopg2.connect('postgresql://Unifai_test:YP2025-2026yp@76.13.243.253:32768/Unifai_test')
cur = conn.cursor()

cur.execute("""
    SELECT id, timestamp, provider, model, virtual_key_name, error_details, routing_rule_name, raw_response
    FROM logs
    WHERE status = 'error'
    ORDER BY timestamp DESC
    LIMIT 10;
""")
rows = cur.fetchall()
print(f"Total error logs found: {len(rows)}")
for r in rows:
    print('---')
    print('ID:', r[0])
    print('Time:', str(r[1]))
    print('Provider:', r[2], '| Model:', r[3], '| VK:', r[4])
    print('Rule:', r[6])
    err_det = (r[5] or '')[:300]
    print('Error Details:', err_det.encode('ascii', 'backslashreplace').decode('ascii'))
    raw_resp = (r[7] or '')[:300]
    print('Raw Response:', raw_resp.encode('ascii', 'backslashreplace').decode('ascii'))

cur.close()
conn.close()
