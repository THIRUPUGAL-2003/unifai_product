import psycopg2

env = {}
with open('.env') as f:
    for line in f:
        line = line.strip()
        if line and not line.startswith('#') and '=' in line:
            k, v = line.split('=', 1)
            env[k.strip()] = v.strip().strip('"').strip("'")

conn = psycopg2.connect(
    host=env['DB_HOST'],
    port=env['DB_PORT'],
    dbname=env['DB_NAME'],
    user=env['DB_USER'],
    password=env['DB_PASSWORD']
)
cur = conn.cursor()
cur.execute("SELECT * FROM browser_target_websites LIMIT 1;")
print("Columns:", [desc[0] for desc in cur.description])
print("Sample row:", cur.fetchone())


conn.close()
