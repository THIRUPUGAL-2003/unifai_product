import psycopg2

env_vars = {}
with open("d:/unifai_project/.env", "r", encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            k, v = line.split("=", 1)
            env_vars[k.strip()] = v.strip().strip("'\"")

conn = psycopg2.connect(
    dbname=env_vars.get("DB_NAME", "unifai"),
    user=env_vars.get("DB_USER", "postgres"),
    password=env_vars.get("DB_PASSWORD", "postgres"),
    host=env_vars.get("DB_HOST", "localhost"),
    port=env_vars.get("DB_PORT", "5432"),
)
cur = conn.cursor()
cur.execute(
    """
    SELECT platform_name, domain, monitored, status, host_role
    FROM browser_target_websites
    WHERE monitored = false OR status <> 'MONITORED'
    ORDER BY platform_name, domain
    """
)
rows = cur.fetchall()
print("not_active", len(rows))
for row in rows:
    print("\t".join(str(x) for x in row))
conn.close()
