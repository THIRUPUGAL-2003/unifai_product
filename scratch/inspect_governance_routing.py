from db_config import get_db_connection
import psycopg2
import json

conn = get_db_connection()
cur = conn.cursor()

print('=== 1. ROUTING RULE ===')
cur.execute('SELECT * FROM routing_rules;')
cols = [d[0] for d in cur.description]
for r in cur.fetchall():
    print(dict(zip(cols, r)))

print('\n=== 2. ROUTING TARGET ===')
cur.execute('SELECT * FROM routing_targets;')
cols = [d[0] for d in cur.description]
for r in cur.fetchall():
    print(dict(zip(cols, r)))

print('\n=== 3. CONFIG PROVIDERS ===')
cur.execute('SELECT * FROM config_providers;')
cols = [d[0] for d in cur.description]
for r in cur.fetchall():
    d = dict(zip(cols, r))
    # mask secret keys
    print({k: v for k, v in d.items() if k != 'keys'})

print('\n=== 4. CONFIG KEYS ===')
cur.execute('SELECT id, provider, name, is_active FROM config_keys;')
cols = [d[0] for d in cur.description]
for r in cur.fetchall():
    print(dict(zip(cols, r)))

print('\n=== 5. VIRTUAL KEY IN RULE (scope_id: 059738dc-e1ff-4914-ab19-e311d1973d1a) ===')
cur.execute("SELECT id, name, key, is_active, max_budget, budget_used FROM governance_virtual_keys;")
cols = [d[0] for d in cur.description]
for r in cur.fetchall():
    print(dict(zip(cols, r)))

cur.close()
conn.close()
