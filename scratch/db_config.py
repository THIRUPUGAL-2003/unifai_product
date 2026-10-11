import os
import psycopg2

def load_env():
    """Load configuration variables from the root .env file."""
    env = {}
    repo_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    env_path = os.path.join(repo_root, ".env")
    if os.path.exists(env_path):
        with open(env_path, "r", encoding="utf-8", errors="ignore") as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    k, v = line.split("=", 1)
                    env[k.strip()] = v.strip().strip("'\"")
    return env

_CONFIG = load_env()

def get_db_url():
    """Get the PostgreSQL connection URL from .env."""
    user = _CONFIG.get("DB_USER", "")
    password = _CONFIG.get("DB_PASSWORD", "")
    host = _CONFIG.get("DB_HOST", "127.0.0.1")
    port = _CONFIG.get("DB_PORT", "5432")
    name = _CONFIG.get("DB_NAME", "")
    ssl = _CONFIG.get("DB_SSL_MODE", "disable")
    return f"postgresql://{user}:{password}@{host}:{port}/{name}?sslmode={ssl}"

def get_db_connection():
    """Return an active psycopg2 connection using .env settings."""
    user = _CONFIG.get("DB_USER", "")
    password = _CONFIG.get("DB_PASSWORD", "")
    host = _CONFIG.get("DB_HOST", "127.0.0.1")
    port = int(_CONFIG.get("DB_PORT", "5432"))
    name = _CONFIG.get("DB_NAME", "")
    ssl = _CONFIG.get("DB_SSL_MODE", "disable")
    return psycopg2.connect(
        host=host,
        port=port,
        dbname=name,
        user=user,
        password=password,
        sslmode=ssl
    )

def get_base_url():
    """Get the Gateway API base URL from .env."""
    port = _CONFIG.get("APP_PORT", "6000")
    return _CONFIG.get("GATEWAY_BACKEND_URL") or f"http://127.0.0.1:{port}"
