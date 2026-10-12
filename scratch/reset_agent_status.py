import psycopg2

conn = psycopg2.connect(
    host="76.13.243.253",
    port=32768,
    dbname="Unifai_test",
    user="Unifai_test",
    password="YP2025-2026yp",
    sslmode="disable"
)
cur = conn.cursor()
cur.execute("""
    UPDATE browser_ai_agents 
    SET status = 'active', health_status = 'ok', health_detail = '', uninstall_requested = false, uninstalled_at = NULL
    WHERE id = 'c792319c-33cb-4d47-a1f8-8f2d16cdb4aa';
""")
conn.commit()
print("Updated count:", cur.rowcount)
cur.execute("SELECT id, hostname, username, status, health_status, health_detail FROM browser_ai_agents WHERE id = 'c792319c-33cb-4d47-a1f8-8f2d16cdb4aa';")
print("Agent record now:", cur.fetchone())
conn.close()
