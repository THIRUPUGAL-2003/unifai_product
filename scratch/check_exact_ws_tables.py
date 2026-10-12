import psycopg2

conn = psycopg2.connect(
    host='76.13.243.253',
    port=32768,
    dbname='Unifai_test',
    user='Unifai_test',
    password='YP2025-2026yp',
    sslmode='disable'
)
cur = conn.cursor()
tables = [
    "access_profiles",
    "rbac_roles",
    "governance_business_units",
    "alert_channels",
    "circuit_breaker_policies",
    "mcp_tool_groups",
    "prompt_deployments",
    "governance_virtual_key_users",
    "governance_virtual_key_teams",
    "governance_virtual_key_customers",
    "governance_virtual_key_business_units",
    "governance_team_members",
    "workspace_settings",
    "audit_logs"
]
missing = []
for t in tables:
    cur.execute(f"SELECT to_regclass('public.{t}')")
    res = cur.fetchone()[0]
    status = 'EXISTS' if res else 'MISSING'
    print(f"{t}: {status}")
    if not res:
        missing.append(t)

print("\nTotal missing:", len(missing), missing)
conn.close()
