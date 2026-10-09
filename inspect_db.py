import sqlite3
import glob

dbs = glob.glob('data/**/*.db', recursive=True) + glob.glob('*.db')
print('Found DBs:', dbs)
for db in dbs:
    try:
        conn = sqlite3.connect(db)
        c = conn.cursor()
        tables = [r[0] for r in c.execute("SELECT name FROM sqlite_master WHERE type='table'").fetchall()]
        print(f"\n--- {db} ---")
        for t in tables:
            rows = c.execute(f"SELECT * FROM [{t}]").fetchall()
            cols = [d[0] for d in c.description]
            print(f"Table: {t} ({len(rows)} rows) cols: {cols}")
            for r in rows[:5]:
                print("  ", r)
        conn.close()
    except Exception as e:
        print(f"Error reading {db}: {e}")
