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
    SELECT platform_name,
           count(*) FILTER (WHERE parent_id IS NULL OR parent_id = '') AS roots,
           count(*) FILTER (WHERE parent_id IS NOT NULL AND parent_id <> '') AS children
    FROM browser_target_websites
    GROUP BY platform_name
    ORDER BY roots DESC, platform_name
    """
)
print("roots\tchildren\tplatform")
for roots, children, name in [(r, c, n) for n, r, c in cur.fetchall()]:
    print(f"{roots}\t{children}\t{name}")
cur.execute(
    "SELECT count(*) FROM browser_target_websites WHERE parent_id IS NULL OR parent_id = ''"
)
print("top_level", cur.fetchone()[0])
conn.close()
