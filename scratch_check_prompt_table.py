import psycopg2

conn = psycopg2.connect(
    host='76.13.243.253',
    port=32768,
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp'
)
cur = conn.cursor()
cur.execute("SELECT table_name FROM information_schema.columns WHERE column_name = 'user_prompt';")
tables = [r[0] for r in cur.fetchall()]
print("Tables with user_prompt:", tables)

for t in tables:
    print(f"\n--- Table: {t} ---")
    cur.execute(f"SELECT column_name FROM information_schema.columns WHERE table_name = '{t}';")
    cols = [r[0] for r in cur.fetchall()]
    print("Columns:", cols)
    cur.execute(f"SELECT * FROM {t} ORDER BY 2 DESC LIMIT 15;")
    for row in cur.fetchall():
        row_dict = dict(zip(cols, row))
        print(f"ID: {row_dict.get('id')} | Time: {row_dict.get('created_at')} | Platform: {row_dict.get('platform')} | User Prompt: {repr(row_dict.get('user_prompt'))}")
        if 'metadata' in row_dict:
            print("  metadata:", str(row_dict.get('metadata'))[:200])

conn.close()
