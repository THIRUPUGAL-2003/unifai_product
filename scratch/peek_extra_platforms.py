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
    SELECT platform_name, domain, host_role, parent_id
    FROM browser_target_websites
    WHERE platform_name IN ('Copilot', 'Microsoft Copilot', 'OpenRouter', 'OpenRouter AI Service', 'Lovable', 'Codeium')
    ORDER BY platform_name, domain
    """
)
for row in cur.fetchall():
    print("\t".join("" if x is None else str(x) for x in row))
conn.close()
