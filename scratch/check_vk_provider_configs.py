import psycopg2

conn = psycopg2.connect('postgresql://Unifai_test:YP2025-2026yp@76.13.243.253:32768/Unifai_test')
cur = conn.cursor()

vk_id = '059738dc-e1ff-4914-ab19-e311d1973d1a'

print('=== 1. governance_virtual_key_provider_configs ===')
cur.execute('SELECT * FROM governance_virtual_key_provider_configs WHERE virtual_key_id = %s;', (vk_id,))
cols = [d[0] for d in cur.description]
for r in cur.fetchall():
    print(dict(zip(cols, r)))

print('\n=== 2. governance_virtual_key_provider_config_keys ===')
cur.execute('SELECT * FROM governance_virtual_key_provider_config_keys WHERE virtual_key_id = %s;', (vk_id,))
cols = [d[0] for d in cur.description]
for r in cur.fetchall():
    print(dict(zip(cols, r)))

print('\n=== 3. ALL config_keys in DB ===')
cur.execute('SELECT id, name, provider, key_id, models_json, blacklisted_models_json, enabled FROM config_keys;')
for r in cur.fetchall():
    print(r)

cur.close()
conn.close()
