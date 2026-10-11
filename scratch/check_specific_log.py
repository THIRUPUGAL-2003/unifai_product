from db_config import get_db_connection
import psycopg2
import json

conn = get_db_connection()
cur = conn.cursor()

cur.execute("""
    SELECT id, routing_rule_id, routing_rule_name, routing_engine_logs, error_details
    FROM logs
    WHERE id = '377f01a9-df05-4acc-b850-ddeea96d204c';
""")
row = cur.fetchone()
print('ID:', row[0])
print('Routing Rule ID:', row[1])
print('Routing Rule Name:', row[2])
if row[3]:
    for entry in json.loads(row[3]) if isinstance(row[3], str) else row[3]:
        print('LOG:', repr(entry))
print('Error details:', row[4])

cur.close()
conn.close()
