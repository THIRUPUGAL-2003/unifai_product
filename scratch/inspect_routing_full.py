from db_config import get_db_connection
import psycopg2
import json

conn = get_db_connection()
cur = conn.cursor()

print('=== 1. ROUTING RULES ===')
cur.execute('SELECT id, name, scope, scope_id, cel_expression, fallbacks, enabled FROM routing_rules;')
for r in cur.fetchall():
    print(r)

print('\n=== 2. ROUTING TARGETS ===')
cur.execute('SELECT rule_id, provider, model, key_id, weight FROM routing_targets;')
for r in cur.fetchall():
    print(r)

print('\n=== 3. PROVIDERS & KEYS IN DB ===')
try:
    cur.execute('SELECT id, name, provider, is_active FROM providers;')
    for r in cur.fetchall():
        print('Provider:', r)
except Exception as e:
    print('Error providers:', e)

try:
    cur.execute('SELECT id, provider, key_name, is_active FROM provider_keys;')
    for r in cur.fetchall():
        print('Provider Key:', r)
except Exception as e:
    print('Error provider_keys:', e)

print('\n=== 4. VIRTUAL KEYS ===')
try:
    cur.execute('SELECT id, name, key, is_active, max_budget, budget_used FROM virtual_keys LIMIT 5;')
    for r in cur.fetchall():
        print('Virtual Key:', r)
except Exception as e:
    print('Error virtual_keys:', e)

cur.close()
conn.close()
