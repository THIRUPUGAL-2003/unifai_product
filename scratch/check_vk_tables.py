import psycopg2

conn = psycopg2.connect('postgresql://Unifai_test:YP2025-2026yp@76.13.243.253:32768/Unifai_test')
cur = conn.cursor()

# Find tables relating to virtual keys
cur.execute("""
    SELECT table_name FROM information_schema.tables 
    WHERE table_schema = 'public' AND table_name LIKE '%virtual_key%';
""")
print('Virtual key tables:', [r[0] for r in cur.fetchall()])

cur.close()
conn.close()
