import re
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
cur.execute("SELECT id, platform_name, domain FROM browser_target_websites")
pat = re.compile(r"^[a-z0-9.-]+$")
bad = []
for rid, platform, domain in cur.fetchall():
    d = (domain or "").strip().lower()
    if not pat.match(d) or ".." in d:
        bad.append((rid, platform, domain))
print("bad", len(bad))
for row in bad:
    print(row)
# delete path-like hosts; the real chat hosts are already stored separately
for rid, platform, domain in bad:
    cur.execute("DELETE FROM browser_target_websites WHERE id = %s", (rid,))
    print("deleted", platform, domain)
conn.commit()
cur.execute("SELECT count(*) FROM browser_target_websites")
print("rows", cur.fetchone()[0])
conn.close()
