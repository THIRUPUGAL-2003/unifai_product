import psycopg2
import sys
from collections import defaultdict

def run_fast_audit():
    conn = psycopg2.connect(
        host="76.13.243.253",
        port=32768,
        dbname="Unifai_test",
        user="Unifai_test",
        password="YP2025-2026yp",
        sslmode="disable"
    )
    cur = conn.cursor()

    print("================================================================================")
    print("                    UNIFAI POSTGRESQL DATABASE COMPLETE AUDIT                   ")
    print("================================================================================\n")

    # 1. Bulk fetch all table row counts from pg_stat_user_tables (or exact count)
    cur.execute("""
        SELECT relname, n_live_tup 
        FROM pg_stat_user_tables 
        WHERE schemaname = 'public'
        ORDER BY relname;
    """)
    stats = dict(cur.fetchall())

    # 2. Bulk fetch all columns
    cur.execute("""
        SELECT table_name, column_name, data_type, is_nullable
        FROM information_schema.columns
        WHERE table_schema = 'public'
        ORDER BY table_name, ordinal_position;
    """)
    table_cols = defaultdict(list)
    for t_name, c_name, d_type, is_null in cur.fetchall():
        table_cols[t_name].append((c_name, d_type, is_null))

    # 3. Bulk fetch primary keys
    cur.execute("""
        SELECT tc.table_name, ccu.column_name
        FROM information_schema.table_constraints tc
        JOIN information_schema.constraint_column_usage ccu 
          ON tc.constraint_name = ccu.constraint_name
          AND tc.table_schema = ccu.table_schema
        WHERE tc.table_schema = 'public' AND tc.constraint_type = 'PRIMARY KEY';
    """)
    table_pks = defaultdict(list)
    for t_name, pk_col in cur.fetchall():
        table_pks[t_name].append(pk_col)

    tables = sorted(table_cols.keys())
    print(f"Total Base Tables in 'public' schema: {len(tables)}\n")

    print(f"{'#':<3} | {'Table Name':<42} | {'Est Rows':<10} | {'Cols':<6} | {'PK':<12}")
    print("-" * 80)
    for idx, t in enumerate(tables, 1):
        rows = stats.get(t, 0)
        cols_count = len(table_cols[t])
        pk_str = ", ".join(table_pks.get(t, [])) if table_pks.get(t) else "NONE"
        print(f"{idx:<3} | {t:<42} | {rows:<10} | {cols_count:<6} | {pk_str:<12}")

    print("\n================================================================================")
    print("                     SPECIFIC SYSTEM INTEGRITY CHECKS                           ")
    print("================================================================================\n")

    # A. Workspace Feature Tables Check
    expected_ws = [
        "access_profiles",
        "rbac_roles",
        "business_units",
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
    print("--- 1. WorkspaceModels Presence ---")
    all_ok = True
    for et in expected_ws:
        if et in tables:
            print(f"  [OK] {et:<40} exists ({len(table_cols[et])} columns)")
        else:
            print(f"  [FAIL] {et:<40} MISSING!")
            all_ok = False

    # B. Core Product Tables Check
    core_tables = [
        "config_users",
        "config_sessions",
        "config_providers",
        "config_keys",
        "config_models",
        "governance_virtual_keys",
        "governance_virtual_key_provider_configs",
        "governance_teams",
        "governance_customers",
        "governance_budgets",
        "prompts",
        "prompt_versions",
        "routing_rules",
        "config_mcp_clients",
        "feature_flags",
        "log_entries"
    ]
    print("\n--- 2. Core Operational Tables Presence ---")
    for ct in core_tables:
        if ct in tables:
            # actual exact row count
            cur.execute(f'SELECT COUNT(*) FROM "{ct}";')
            cnt = cur.fetchone()[0]
            print(f"  [OK] {ct:<42} exists (Exact Rows: {cnt}, Cols: {len(table_cols[ct])})")
        else:
            print(f"  [FAIL] {ct:<42} MISSING!")
            all_ok = False

    # C. Data & Consistency Check
    print("\n--- 3. Relational Consistency & Orphan Audit ---")

    # Virtual Keys & Relationships
    cur.execute("SELECT COUNT(*) FROM governance_virtual_keys;")
    vk_count = cur.fetchone()[0]
    print(f"  * Total Virtual Keys: {vk_count}")

    # Orphaned joins
    for join_tbl, fk_col, parent_tbl, pk_col in [
        ("governance_virtual_key_users", "virtual_key_id", "governance_virtual_keys", "id"),
        ("governance_virtual_key_teams", "virtual_key_id", "governance_virtual_keys", "id"),
        ("governance_virtual_key_customers", "virtual_key_id", "governance_virtual_keys", "id"),
        ("governance_virtual_key_business_units", "virtual_key_id", "governance_virtual_keys", "id"),
        ("prompt_versions", "prompt_id", "prompts", "id"),
        ("governance_team_members", "team_id", "governance_teams", "id"),
        ("governance_virtual_key_provider_configs", "virtual_key_id", "governance_virtual_keys", "id"),
    ]:
        if join_tbl in tables and parent_tbl in tables:
            cur.execute(f"""
                SELECT COUNT(*) FROM "{join_tbl}"
                WHERE "{fk_col}" NOT IN (SELECT "{pk_col}" FROM "{parent_tbl}");
            """)
            orphans = cur.fetchone()[0]
            status = "[OK] 0 orphans" if orphans == 0 else f"[WARNING] {orphans} orphaned records!"
            print(f"  * {join_tbl}.{fk_col} -> {parent_tbl}.{pk_col}: {status}")

    # Users sanity
    print("\n--- 4. Users & Roles Audit ---")
    cur.execute("SELECT username, role, status FROM config_users;")
    users = cur.fetchall()
    for u in users:
        print(f"  * User: {u[0]:<20} | Role: {u[1]:<12} | Status: {u[2]}")

    conn.close()
    print("\nAudit finished successfully.")

if __name__ == "__main__":
    run_fast_audit()
