import sqlite3
c = sqlite3.connect(r"d:\unifai_project\transports\gateway-http\data\config.db")
tables = [r[0] for r in c.execute("SELECT name FROM sqlite_master WHERE type='table' ORDER BY 1")]
print("tables", len(tables))
want = [t for t in tables if any(x in t for x in ("virtual", "routing", "team", "customer", "mcp", "user"))]
print("\n".join(want))
