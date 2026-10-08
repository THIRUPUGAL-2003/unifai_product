import os
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
conn = psycopg2.connect(
    dbname=env.get("DB_NAME", "Unifai_test"),
    user=env.get("DB_USER", "postgres"),
    password=env.get("DB_PASSWORD", ""),
    host=env.get("DB_HOST", "76.13.243.253"),
    port=env.get("DB_PORT", "32768")
)
cur = conn.cursor(cursor_factory=RealDictCursor)

cur.execute("SELECT column_name, data_type FROM information_schema.columns WHERE table_name = 'access_profiles'")
cols = cur.fetchall()
print("access_profiles columns:")
for c in cols:
    print(f" - {c['column_name']} ({c['data_type']})")

conn.close()
