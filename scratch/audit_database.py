import psycopg2
import sys
import json

def audit():
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

    # 1. Fetch all tables
    cur.execute("""
        SELECT table_name 
        FROM information_schema.tables 
        WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
        ORDER BY table_name;
    """)
    tables = [r[0] for r in cur.fetchall()]
    print(f"Total Tables in 'public' schema: {len(tables)}\n")

    table_stats = []
    
    # 2. Iterate each table, get row counts, columns, constraints
    for t in tables:
        # row count
        try:
            cur.execute(f'SELECT COUNT(*) FROM "{t}";')
            cnt = cur.fetchone()[0]
        except Exception as e:
            cnt = -1

        # columns
        cur.execute("""
            SELECT column_name, data_type, is_nullable, column_default
            FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = %s
            ORDER BY ordinal_position;
        """, (t,))
        cols = cur.fetchall()

        # primary key
        cur.execute("""
            SELECT c.column_name
            FROM information_schema.table_constraints tc
            JOIN information_schema.constraint_column_usage AS ccu USING (constraint_schema, constraint_name)
            JOIN information_schema.columns AS c ON c.table_schema = tc.constraint_schema
              AND tc.table_name = c.table_name AND ccu.column_name = c.column_name
            WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_name = %s;
        """, (t,))
        pks = [r[0] for r in cur.fetchall()]

        # foreign keys
        cur.execute("""
            SELECT
                kcu.column_name,
                ccu.table_name AS foreign_table_name,
                ccu.column_name AS foreign_column_name
            FROM information_schema.table_constraints AS tc
            JOIN information_schema.key_column_usage AS kcu
              ON tc.constraint_name = kcu.constraint_name
              AND tc.table_schema = kcu.table_schema
            JOIN information_schema.constraint_column_usage AS ccu
              ON ccu.constraint_name = tc.constraint_name
              AND ccu.table_schema = tc.table_schema
            WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_name = %s;
        """, (t,))
        fks = cur.fetchall()

        # indexes
        cur.execute("""
            SELECT indexname, indexdef
            FROM pg_indexes
            WHERE schemaname = 'public' AND tablename = %s;
        """, (t,))
        idxs = cur.fetchall()

        table_stats.append({
            "name": t,
            "rows": cnt,
            "cols_count": len(cols),
            "columns": cols,
            "pks": pks,
            "fks": fks,
            "indexes": idxs
        })

    # Print summary table
    print(f"{'#':<3} | {'Table Name':<42} | {'Rows':<8} | {'Columns':<8} | {'PK':<12}")
    print("-" * 80)
    for i, s in enumerate(table_stats, 1):
        pk_str = ", ".join(s['pks']) if s['pks'] else "NONE"
        print(f"{i:<3} | {s['name']:<42} | {s['rows']:<8} | {s['cols_count']:<8} | {pk_str:<12}")

    print("\n================================================================================")
    print("                     SPECIFIC INTEGRITY & RELATION CHECKS                       ")
    print("================================================================================\n")

    # A. Check Virtual Keys & Join Tables
    print("--- A. Virtual Keys & Join Tables ---")
    cur.execute("SELECT id, name, is_active, value FROM governance_virtual_keys;")
    vks = cur.fetchall()
    print(f"Total Virtual Keys: {len(vks)}")
    for v in vks:
        vk_id = v[0]
        # check user join
        cur.execute("SELECT user_id FROM governance_virtual_key_users WHERE virtual_key_id = %s;", (vk_id,))
        u_joins = cur.fetchall()
        # check team join
        cur.execute("SELECT team_id FROM governance_virtual_key_teams WHERE virtual_key_id = %s;", (vk_id,))
        t_joins = cur.fetchall()
        # check customer join
        cur.execute("SELECT customer_id FROM governance_virtual_key_customers WHERE virtual_key_id = %s;", (vk_id,))
        c_joins = cur.fetchall()
        # check bu join
        cur.execute("SELECT business_unit_id FROM governance_virtual_key_business_units WHERE virtual_key_id = %s;", (vk_id,))
        bu_joins = cur.fetchall()
        print(f"  * VK: '{v[1]}' (Active={v[2]}): {len(u_joins)} users, {len(t_joins)} teams, {len(c_joins)} customers, {len(bu_joins)} BUs")

    # B. Check Users & Sessions
    print("\n--- B. Users & Authentication ---")
    cur.execute("SELECT id, username, email, role, status FROM config_users;")
    users = cur.fetchall()
    print(f"Total Users: {len(users)}")
    for u in users:
        print(f"  * User: '{u[1]}' (Role={u[3]}, Status={u[4]}, Email={u[2]})")

    # C. Check Providers & DB Keys
    print("\n--- C. Configured Providers & Keys ---")
    cur.execute("SELECT id, name FROM config_providers;")
    provs = cur.fetchall()
    print(f"Total Providers: {len(provs)} -> {[p[1] for p in provs]}")
    cur.execute("SELECT key_id, name, provider FROM config_keys;")
    keys = cur.fetchall()
    print(f"Total Raw API Keys: {len(keys)}")
    for k in keys:
        print(f"  * Raw Key: '{k[1]}' for Provider '{k[2]}'")

    # D. Check Prompts & Prompt Versions
    print("\n--- D. Prompt Repository ---")
    cur.execute("SELECT id, name FROM prompts;")
    prompts = cur.fetchall()
    print(f"Total Prompts: {len(prompts)}")
    for p in prompts:
        cur.execute("SELECT COUNT(*) FROM prompt_versions WHERE prompt_id = %s;", (p[0],))
        v_cnt = cur.fetchone()[0]
        print(f"  * Prompt: '{p[1]}' ({v_cnt} versions)")

    # E. Check Guardrails
    print("\n--- E. Guardrails ---")
    cur.execute("SELECT table_name FROM information_schema.tables WHERE table_name LIKE '%guardrail%';")
    gr_tables = [r[0] for r in cur.fetchall()]
    print(f"Guardrail related tables: {gr_tables}")
    for gt in gr_tables:
        cur.execute(f"SELECT COUNT(*) FROM \"{gt}\";")
        print(f"  * {gt}: {cur.fetchone()[0]} rows")

    # F. Check Routing Rules
    print("\n--- F. Routing Rules ---")
    cur.execute("SELECT COUNT(*) FROM routing_rules;")
    print(f"Routing Rules count: {cur.fetchone()[0]}")

    # G. Check Workspace Feature Tables defined in WorkspaceModels
    print("\n--- G. WorkspaceModels Verification ---")
    expected_workspace_tables = [
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
    missing_ws = []
    for et in expected_workspace_tables:
        if et in tables:
            print(f"  [OK] {et} exists")
        else:
            print(f"  [MISSING] {et} DOES NOT EXIST!")
            missing_ws.append(et)

    # H. Check orphaned records
    print("\n--- H. Orphaned Foreign Keys Audit ---")
    # virtual key users orphans
    cur.execute("""
        SELECT COUNT(*) FROM governance_virtual_key_users 
        WHERE virtual_key_id NOT IN (SELECT id FROM governance_virtual_keys);
    """)
    orphan_vku = cur.fetchone()[0]
    print(f"Orphaned governance_virtual_key_users -> virtual_key_id: {orphan_vku}")

    # virtual key teams orphans
    cur.execute("""
        SELECT COUNT(*) FROM governance_virtual_key_teams 
        WHERE virtual_key_id NOT IN (SELECT id FROM governance_virtual_keys);
    """)
    orphan_vkt = cur.fetchone()[0]
    print(f"Orphaned governance_virtual_key_teams -> virtual_key_id: {orphan_vkt}")

    # virtual key bu orphans
    cur.execute("""
        SELECT COUNT(*) FROM governance_virtual_key_business_units 
        WHERE virtual_key_id NOT IN (SELECT id FROM governance_virtual_keys);
    """)
    orphan_vkbu = cur.fetchone()[0]
    print(f"Orphaned governance_virtual_key_business_units -> virtual_key_id: {orphan_vkbu}")

    # prompt versions orphans
    cur.execute("""
        SELECT COUNT(*) FROM prompt_versions 
        WHERE prompt_id NOT IN (SELECT id FROM prompts);
    """)
    orphan_pv = cur.fetchone()[0]
    print(f"Orphaned prompt_versions -> prompt_id: {orphan_pv}")

    # team members orphans
    cur.execute("""
        SELECT COUNT(*) FROM governance_team_members 
        WHERE team_id NOT IN (SELECT id FROM governance_teams);
    """)
    orphan_tm = cur.fetchone()[0]
    print(f"Orphaned governance_team_members -> team_id: {orphan_tm}")

    conn.close()
    print("\nAudit completed successfully.")

if __name__ == "__main__":
    audit()
