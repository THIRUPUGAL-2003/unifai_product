import psycopg2

conn = psycopg2.connect('postgresql://Unifai_test:YP2025-2026yp@76.13.243.253:32768/Unifai_test')
cur = conn.cursor()

print('=== 4. CONFIG KEYS ===')
cur.execute('SELECT * FROM config_keys;')
cols = [d[0] for d in cur.description]
for r in cur.fetchall():
    d = dict(zip(cols, r))
    # mask key
    if 'key' in d:
        d['key'] = d['key'][:8] + '...' if d['key'] else None
    print(d)

print('\n=== 5. VIRTUAL KEYS ===')
cur.execute('SELECT * FROM governance_virtual_keys;')
cols = [d[0] for d in cur.description]
for r in cur.fetchall():
    d = dict(zip(cols, r))
    if 'key' in d:
        d['key'] = d['key'][:10] + '...' if d['key'] else None
    print(d)

cur.close()
conn.close()
