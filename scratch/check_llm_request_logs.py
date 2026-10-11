import psycopg2

conn = psycopg2.connect('postgresql://Unifai_test:YP2025-2026yp@76.13.243.253:32768/Unifai_test')
cur = conn.cursor()

cur.execute("SELECT column_name FROM information_schema.columns WHERE table_name = 'logs' ORDER BY ordinal_position;")
cols = [r[0] for r in cur.fetchall()]
print('Columns count:', len(cols))
print('Columns:', cols)

cur.execute("SELECT * FROM logs ORDER BY timestamp DESC LIMIT 5;")
rows = cur.fetchall()
for row in rows:
    d = dict(zip(cols, row))
    # print relevant fields
    print({
        'id': d.get('id'),
        'timestamp': str(d.get('timestamp')),
        'provider': d.get('provider'),
        'model': d.get('model'),
        'status': d.get('status'),
        'status_code': d.get('status_code'),
        'routing_rule_id': d.get('routing_rule_id'),
        'routing_rule_name': d.get('routing_rule_name'),
        'routing_engines_used': d.get('routing_engines_used'),
        'virtual_key_name': d.get('virtual_key_name'),
        'error': d.get('error'),
        'error_response': d.get('error_response') or d.get('error_json') or d.get('error_message')
    })

cur.close()
conn.close()
