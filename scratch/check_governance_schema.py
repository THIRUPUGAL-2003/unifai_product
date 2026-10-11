import psycopg2

env_vars = {}
with open('d:/unifai_project/.env', 'r', encoding='utf-8') as f:
    for line in f:
        line = line.strip()
        if line and not line.startswith('#') and '=' in line:
            k, v = line.split('=', 1)
            env_vars[k.strip()] = v.strip().strip('\'"')

conn = psycopg2.connect(
    dbname=env_vars.get('DB_NAME', 'unifai'),
    user=env_vars.get('DB_USER', 'postgres'),
    password=env_vars.get('DB_PASSWORD', 'postgres'),
    host=env_vars.get('DB_HOST', 'localhost'),
    port=env_vars.get('DB_PORT', '5432')
)
cur = conn.cursor()
tables = [
    'governance_customers',
    'governance_teams',
    'governance_team_members',
    'governance_users',
    'governance_virtual_keys',
    'governance_budgets',
    'governance_rate_limits'
]
for t in tables:
    cur.execute(
        "SELECT column_name, data_type FROM information_schema.columns WHERE table_name = %s ORDER BY ordinal_position",
        (t,)
    )
    cols = cur.fetchall()
    print(f"=== {t} ({len(cols)} columns) ===")
    for c in cols:
        print(f"  - {c[0]}: {c[1]}")
    print()

conn.close()
