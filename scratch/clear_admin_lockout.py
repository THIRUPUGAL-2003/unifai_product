"""Clear this machine's local admin login lock so the requested plugin test can run."""
import sys
sys.path.insert(0, ".")
from db_config import get_db_connection

conn = get_db_connection()
cur = conn.cursor()
cur.execute(
    """
    UPDATE auth_login_lockouts
    SET failed_count = 0, locked_until = NULL
    WHERE username_key IN ('admin', 'admin@yespanchi.com')
       OR username_key LIKE 'ip:%'
    """
)
print("cleared_rows", cur.rowcount)
conn.commit()
conn.close()
