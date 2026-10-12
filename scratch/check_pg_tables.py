import psycopg2

def check_db():
    conn = psycopg2.connect(
        host='76.13.243.253',
        port=32768,
        dbname='Unifai_test',
        user='Unifai_test',
        password='YP2025-2026yp',
        sslmode='disable'
    )
    cur = conn.cursor()
    cur.execute("SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name LIKE 'governance_%' ORDER BY table_name")
    print("Governance tables in DB:")
    for row in cur.fetchall():
        print(" -", row[0])

    cur.execute("SELECT to_regclass('public.governance_virtual_key_business_units')")
    print("\nHas governance_virtual_key_business_units:", cur.fetchone())

    # Check migration records
    cur.execute("SELECT id FROM migrations ORDER BY id")
    print("\nTotal migrations:", len(cur.fetchall()))
    conn.close()

if __name__ == '__main__':
    check_db()
