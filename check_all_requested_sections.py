import os
import json
import psycopg2
from psycopg2.extras import RealDictCursor

def load_env(env_path=".env"):
    env_vars = {}
    if os.path.exists(env_path):
        with open(env_path, 'r', encoding='utf-8') as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith('#') and '=' in line:
                    k, v = line.split('=', 1)
                    env_vars[k.strip()] = v.strip().strip('"').strip("'")
    return env_vars

env = load_env()
DB_NAME = env.get("DB_NAME", "Unifai_test")
DB_USER = env.get("DB_USER", "postgres")
DB_PASSWORD = env.get("DB_PASSWORD", "")
DB_HOST = env.get("DB_HOST", "76.13.243.253")
DB_PORT = env.get("DB_PORT", "32768")

conn = psycopg2.connect(
    dbname=DB_NAME,
    user=DB_USER,
    password=DB_PASSWORD,
    host=DB_HOST,
    port=DB_PORT,
    connect_timeout=10
)
cur = conn.cursor(cursor_factory=RealDictCursor)

def print_table_info(title, table_name):
    print(f"\n--- {title} [{table_name}] ---")
    try:
        cur.execute(f"SELECT column_name, data_type FROM information_schema.columns WHERE table_name = '{table_name}' ORDER BY ordinal_position")
        cols = cur.fetchall()
        print(f"Columns: {', '.join([c['column_name'] for c in cols])}")
        cur.execute(f"SELECT count(*) FROM {table_name}")
        cnt = cur.fetchone()['count']
        print(f"Total Rows: {cnt}")
        if cnt > 0:
            cur.execute(f"SELECT * FROM {table_name} LIMIT 3")
            for r in cur.fetchall():
                # sanitize values
                sanitized = {}
                for k, v in r.items():
                    if isinstance(v, (dict, list)):
                        sanitized[k] = json.dumps(v)[:100]
                    else:
                        sanitized[k] = str(v)[:100]
                print(f" Sample: {sanitized}")
    except Exception as e:
        conn.rollback()
        print(f" Error: {e}")

print("================== 1. PLUGINS ==================")
print_table_info("Plugins Configuration", "config_plugins")

print("\n================== 2. SKILL REPOSITORY ==================")
for st in ['skills', 'skill_versions', 'skill_files', 'skill_file_blobs']:
    print_table_info(f"Skill Table: {st}", st)

print("\n================== 3. SETTINGS & HEADERS ==================")
for st in ['workspace_settings', 'mcp_per_user_header_credentials', 'mcp_per_user_header_flows', 'browser_control_settings', 'browser_ai_agent_settings']:
    print_table_info(f"Settings Table: {st}", st)

print("\n================== 4. GUARDRAILS ==================")
for gt in ['guardrails', 'browser_guard_rules']:
    print_table_info(f"Guardrails Table: {gt}", gt)

print("\n================== 5. GOVERNANCE HIERARCHY ==================")
for gov in [
    'governance_users',
    'governance_teams',
    'governance_team_members',
    'governance_business_units',
    'governance_customers',
    'governance_virtual_keys',
    'access_profiles',
    'rbac_roles',
    'audit_logs'
]:
    print_table_info(f"Gov: {gov}", gov)

print("\n================== 6. OBSERVABILITY & CONNECTORS ==================")
# check for alerts, connectors, metrics, telemetry
cur.execute("SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND (table_name LIKE '%alert%' OR table_name LIKE '%log%' OR table_name LIKE '%metric%' OR table_name LIKE '%audit%')")
for r in cur.fetchall():
    print_table_info(f"Observability Table: {r['table_name']}", r['table_name'])

conn.close()
