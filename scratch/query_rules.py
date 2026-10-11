from db_config import get_db_connection
import psycopg2

conn = get_db_connection()
cur = conn.cursor()

cur.execute("""
    SELECT table_name 
    FROM information_schema.tables 
    WHERE table_schema = 'public' 
      AND (table_name LIKE '%rule%' OR table_name LIKE '%browser%' OR table_name LIKE '%guard%' OR table_name LIKE '%routing%');
""")
tables = [r[0] for r in cur.fetchall()]
print("Matching tables:", tables)

# Query routing_rules
if 'routing_rules' in tables:
    cur.execute("SELECT * FROM routing_rules;")
    cols = [d[0] for d in cur.description]
    print("\n--- routing_rules records ---")
    for r in cur.fetchall():
        print(dict(zip(cols, r)))

# Query routing_targets
if 'routing_targets' in tables:
    cur.execute("SELECT * FROM routing_targets;")
    cols = [d[0] for d in cur.description]
    print("\n--- routing_targets records ---")
    for r in cur.fetchall():
        print(dict(zip(cols, r)))

# Check any other rule table
for t in tables:
    if 'rule' in t and t != 'routing_rules':
        cur.execute(f"SELECT * FROM {t} LIMIT 5;")
        cols = [d[0] for d in cur.description]
        print(f"\n--- {t} records ---")
        for r in cur.fetchall():
            print(dict(zip(cols, r)))

cur.close()
conn.close()
