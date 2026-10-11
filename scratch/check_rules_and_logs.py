import psycopg2

conn = psycopg2.connect('postgresql://postgres:postgres@localhost:5432/unifai')
cur = conn.cursor()
cur.execute("SELECT table_name FROM information_schema.tables WHERE table_schema='public'")
tables = [r[0] for r in cur.fetchall()]
print("Tables in public schema:", [t for t in tables if 'browser' in t or 'rule' in t or 'log' in t or 'ai' in t])
