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
cur.execute("""
    SELECT column_name, data_type, is_nullable
    FROM information_schema.columns
    WHERE table_name = 'governance_virtual_key_teams'
    ORDER BY ordinal_position
""")
print("governance_virtual_key_teams columns:")
for row in cur.fetchall():
    print(" -", row)

# Now create governance_virtual_key_business_units
create_sql = """
CREATE TABLE IF NOT EXISTS public.governance_virtual_key_business_units (
    id bigserial PRIMARY KEY,
    virtual_key_id varchar(255) NOT NULL,
    business_unit_id varchar(255) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_vk_bu_pair ON public.governance_virtual_key_business_units (virtual_key_id, business_unit_id);
CREATE INDEX IF NOT EXISTS idx_vkbu_vk_id ON public.governance_virtual_key_business_units (virtual_key_id);
CREATE INDEX IF NOT EXISTS idx_vkbu_bu_id ON public.governance_virtual_key_business_units (business_unit_id);
"""
cur.execute(create_sql)
conn.commit()

cur.execute("SELECT to_regclass('public.governance_virtual_key_business_units')")
print("\nCreated table:", cur.fetchone())

conn.close()
