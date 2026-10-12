import psycopg2

def check_missing_tables():
    conn = psycopg2.connect(
        host='76.13.243.253',
        port=32768,
        dbname='Unifai_test',
        user='Unifai_test',
        password='YP2025-2026yp',
        sslmode='disable'
    )
    cur = conn.cursor()
    workspace_tables = [
        "workspace_access_profiles",
        "workspace_rbac_roles",
        "governance_business_units",
        "workspace_alert_channels",
        "workspace_circuit_breaker_policies",
        "workspace_mcp_tool_groups",
        "workspace_prompt_deployments",
        "governance_virtual_key_users",
        "governance_virtual_key_teams",
        "governance_virtual_key_customers",
        "governance_virtual_key_business_units",
        "governance_team_members",
        "workspace_settings",
        "workspace_audit_logs"
    ]
    for t in workspace_tables:
        cur.execute(f"SELECT to_regclass('public.{t}')")
        res = cur.fetchone()[0]
        print(f"{t}: {'EXISTS' if res else 'MISSING'}")
    conn.close()

if __name__ == '__main__':
    check_missing_tables()
