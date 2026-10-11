import psycopg2

conn = psycopg2.connect(
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    host='76.13.243.253',
    port='32768'
)
cur = conn.cursor()
cur.execute("SELECT current_database(), current_schema(), count(*) FROM browser_ai_logs;")
print("Unifai_test count:", cur.fetchall())

# List all databases
cur.execute("SELECT datname FROM pg_database WHERE datistemplate = false;")
dbs = [r[0] for r in cur.fetchall()]

for db in dbs:
    try:
        c = psycopg2.connect(
            dbname=db,
            user='Dev_yp',
            password='YP2025-2026yp',
            host='76.13.243.253',
            port='32768'
        )
        cu = c.cursor()
        cu.execute("SELECT to_regclass('public.browser_ai_logs');")
        if cu.fetchone()[0]:
            cu.execute("SELECT count(*) FROM browser_ai_logs;")
            cnt = cu.fetchone()[0]
            print(f"DB {db}: {cnt} rows")
            if cnt > 0:
                cu.execute("SELECT created_at, platform, user_prompt_preview, action, rule_triggered FROM browser_ai_logs ORDER BY created_at DESC LIMIT 5;")
                for row in cu.fetchall():
                    print("  ", row)
        c.close()
    except Exception as e:
        # print(f"Error for {db}: {e}")
        pass
