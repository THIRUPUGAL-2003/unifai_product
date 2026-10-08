#!/usr/bin/env python3
"""Governance Full Connection & End-to-End Test Suite.
Validates the complete connectivity and integrity of all 9 Governance modules:
1. Virtual Keys
2. Users
3. Teams
4. Business Units
5. Customers
6. User Provisioning (SCIM 2.0)
7. Roles & Permissions (RBAC)
8. Access Profiles
9. Audit Logs
"""

import json
import os
import re
import sys
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[4]
UI_DIR = REPO_ROOT / "ui"
HANDLERS_DIR = REPO_ROOT / "transports" / "gateway-http" / "handlers"


class GovernanceConnectionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        import psycopg2
        import psycopg2.extras

        # Connect using live DB credentials
        cls.conn = psycopg2.connect(
            host="76.13.243.253",
            port=32768,
            dbname="Unifai_test",
            user="Unifai_test",
            password="YP2025-2026yp",
            sslmode="disable",
            connect_timeout=10,
        )
        cls.cur = cls.conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

    @classmethod
    def tearDownClass(cls):
        cls.cur.close()
        cls.conn.close()

    # -------------------------------------------------------------------------
    # 1. Virtual Keys Connection
    # -------------------------------------------------------------------------
    def test_01_virtual_keys_connection(self):
        """Verify Virtual Keys DB tables, API handlers, and UI routes."""
        # 1. DB tables
        self.cur.execute("SELECT count(*) FROM governance_virtual_keys")
        vk_count = self.cur.fetchone()["count"]
        self.assertGreaterEqual(vk_count, 1)

        self.cur.execute("SELECT count(*) FROM governance_virtual_key_users")
        self.assertGreaterEqual(self.cur.fetchone()["count"], 1)

        # 2. Backend routes
        gov_go = (HANDLERS_DIR / "governance.go").read_text(encoding="utf-8")
        self.assertIn('r.GET("/api/governance/virtual-keys"', gov_go)
        self.assertIn('r.POST("/api/governance/virtual-keys"', gov_go)
        self.assertIn('r.GET("/api/governance/virtual-keys/{vk_id}"', gov_go)
        self.assertIn('r.PUT("/api/governance/virtual-keys/{vk_id}"', gov_go)
        self.assertIn('r.DELETE("/api/governance/virtual-keys/{vk_id}"', gov_go)

        # 3. UI route
        sidebar_ts = (UI_DIR / "components" / "sidebar.tsx").read_text(encoding="utf-8")
        self.assertIn('/workspace/governance/virtual-keys', sidebar_ts)

    # -------------------------------------------------------------------------
    # 2. Users Connection
    # -------------------------------------------------------------------------
    def test_02_users_connection(self):
        """Verify Users DB tables, roles, API handlers, and UI routes."""
        # 1. DB table & records
        self.cur.execute("SELECT count(*) AS total_users, count(DISTINCT role) AS total_roles FROM governance_users")
        res = self.cur.fetchone()
        self.assertGreaterEqual(res["total_users"], 5)
        self.assertGreaterEqual(res["total_roles"], 2)

        # 2. Backend routes in session.go and workspace.go
        session_go = (HANDLERS_DIR / "session.go").read_text(encoding="utf-8")
        self.assertIn('r.GET("/api/session/users"', session_go)
        self.assertIn('r.POST("/api/session/users"', session_go)
        self.assertIn('r.PUT("/api/session/users/{id}"', session_go)
        self.assertIn('r.DELETE("/api/session/users/{id}"', session_go)

        workspace_go = (HANDLERS_DIR / "workspace.go").read_text(encoding="utf-8")
        self.assertIn('r.PUT("/api/users/{id}/role"', workspace_go)

        # 3. UI route
        sidebar_ts = (UI_DIR / "components" / "sidebar.tsx").read_text(encoding="utf-8")
        self.assertIn('/workspace/governance/users', sidebar_ts)

    # -------------------------------------------------------------------------
    # 3. Teams Connection
    # -------------------------------------------------------------------------
    def test_03_teams_connection(self):
        """Verify Teams DB tables, membership linkage, API handlers, and UI routes."""
        # 1. DB table & membership linkage
        self.cur.execute("SELECT count(*) FROM governance_teams")
        self.assertGreaterEqual(self.cur.fetchone()["count"], 1)

        self.cur.execute("SELECT count(*) FROM governance_team_members")
        self.assertGreaterEqual(self.cur.fetchone()["count"], 1)

        # 2. Backend routes
        gov_go = (HANDLERS_DIR / "governance.go").read_text(encoding="utf-8")
        self.assertIn('r.GET("/api/governance/teams"', gov_go)
        self.assertIn('r.POST("/api/governance/teams"', gov_go)
        self.assertIn('r.GET("/api/governance/teams/{team_id}"', gov_go)
        self.assertIn('r.PUT("/api/governance/teams/{team_id}"', gov_go)
        self.assertIn('r.DELETE("/api/governance/teams/{team_id}"', gov_go)
        self.assertIn('r.GET("/api/governance/teams/{team_id}/members"', gov_go)
        self.assertIn('r.POST("/api/governance/teams/{team_id}/members"', gov_go)

        # 3. UI route
        sidebar_ts = (UI_DIR / "components" / "sidebar.tsx").read_text(encoding="utf-8")
        self.assertIn('/workspace/governance/teams', sidebar_ts)

    # -------------------------------------------------------------------------
    # 4. Business Units Connection
    # -------------------------------------------------------------------------
    def test_04_business_units_connection(self):
        """Verify Business Units DB table, team linking, API handlers, and UI routes."""
        # 1. DB table
        self.cur.execute("SELECT count(*) FROM governance_business_units")
        self.assertGreaterEqual(self.cur.fetchone()["count"], 1)

        # 2. Backend routes in workspace.go
        ws_go = (HANDLERS_DIR / "workspace.go").read_text(encoding="utf-8")
        self.assertIn('r.GET("/api/governance/business-units"', ws_go)
        self.assertIn('r.POST("/api/governance/business-units"', ws_go)
        self.assertIn('r.GET("/api/governance/business-units/{id}"', ws_go)
        self.assertIn('r.DELETE("/api/governance/business-units/{id}"', ws_go)
        self.assertIn('r.GET("/api/governance/business-units/{id}/teams"', ws_go)
        self.assertIn('r.POST("/api/governance/business-units/{id}/teams"', ws_go)
        self.assertIn('r.DELETE("/api/governance/business-units/{id}/teams/{team_id}"', ws_go)

        # 3. UI route
        sidebar_ts = (UI_DIR / "components" / "sidebar.tsx").read_text(encoding="utf-8")
        self.assertIn('/workspace/governance/business-units', sidebar_ts)

    # -------------------------------------------------------------------------
    # 5. Customers Connection
    # -------------------------------------------------------------------------
    def test_05_customers_connection(self):
        """Verify Customers DB table, API handlers, and UI routes."""
        # 1. DB table
        self.cur.execute("SELECT count(*) FROM governance_customers")
        self.assertGreaterEqual(self.cur.fetchone()["count"], 1)

        # 2. Backend routes
        gov_go = (HANDLERS_DIR / "governance.go").read_text(encoding="utf-8")
        self.assertIn('r.GET("/api/governance/customers"', gov_go)
        self.assertIn('r.POST("/api/governance/customers"', gov_go)
        self.assertIn('r.GET("/api/governance/customers/{customer_id}"', gov_go)
        self.assertIn('r.PUT("/api/governance/customers/{customer_id}"', gov_go)
        self.assertIn('r.DELETE("/api/governance/customers/{customer_id}"', gov_go)

        # 3. UI route
        sidebar_ts = (UI_DIR / "components" / "sidebar.tsx").read_text(encoding="utf-8")
        self.assertIn('/workspace/governance/customers', sidebar_ts)

    # -------------------------------------------------------------------------
    # 6. User Provisioning (SCIM 2.0) Connection
    # -------------------------------------------------------------------------
    def test_06_user_provisioning_connection(self):
        """Verify User Provisioning workspace_settings, SCIM routes, and UI views."""
        # 1. DB table (workspace_settings has scim setting)
        self.cur.execute("SELECT count(*) FROM workspace_settings WHERE key = 'scim'")
        self.assertGreaterEqual(self.cur.fetchone()["count"], 1)

        # 2. Backend routes
        ws_go = (HANDLERS_DIR / "workspace.go").read_text(encoding="utf-8")
        self.assertIn('r.GET("/api/scim/config"', ws_go)
        self.assertIn('r.PUT("/api/scim/config"', ws_go)
        self.assertIn('r.GET("/scim/v2/ServiceProviderConfig"', ws_go)
        self.assertIn('r.GET("/scim/v2/Users"', ws_go)
        self.assertIn('r.POST("/scim/v2/Users"', ws_go)
        self.assertIn('r.GET("/scim/v2/Groups"', ws_go)

        # 3. UI route
        sidebar_ts = (UI_DIR / "components" / "sidebar.tsx").read_text(encoding="utf-8")
        self.assertIn('/workspace/scim', sidebar_ts)

    # -------------------------------------------------------------------------
    # 7. Roles & Permissions (RBAC) Connection
    # -------------------------------------------------------------------------
    def test_07_roles_and_permissions_connection(self):
        """Verify RBAC roles table, permission routes, and UI views."""
        # 1. DB table (rbac_roles has system roles)
        self.cur.execute("SELECT count(*) FROM rbac_roles")
        self.assertGreaterEqual(self.cur.fetchone()["count"], 3)

        # 2. Backend routes
        ws_go = (HANDLERS_DIR / "workspace.go").read_text(encoding="utf-8")
        self.assertIn('r.GET("/api/roles"', ws_go)
        self.assertIn('r.POST("/api/roles"', ws_go)
        self.assertIn('r.GET("/api/roles/{id}"', ws_go)
        self.assertIn('r.PUT("/api/roles/{id}"', ws_go)
        self.assertIn('r.DELETE("/api/roles/{id}"', ws_go)
        self.assertIn('r.GET("/api/permissions"', ws_go)
        self.assertIn('r.GET("/api/rbac/me/permissions"', ws_go)

        # 3. UI route
        sidebar_ts = (UI_DIR / "components" / "sidebar.tsx").read_text(encoding="utf-8")
        self.assertIn('/workspace/governance/rbac', sidebar_ts)

    # -------------------------------------------------------------------------
    # 8. Access Profiles Connection
    # -------------------------------------------------------------------------
    def test_08_access_profiles_connection(self):
        """Verify Access Profiles table, handlers, and UI views."""
        # 1. DB table
        self.cur.execute("SELECT count(*) FROM access_profiles")
        # Table exists and is queryable
        self.assertGreaterEqual(self.cur.fetchone()["count"], 0)

        # 2. Backend routes
        ws_go = (HANDLERS_DIR / "workspace.go").read_text(encoding="utf-8")
        self.assertIn('r.GET("/api/access-profiles"', ws_go)
        self.assertIn('r.POST("/api/access-profiles"', ws_go)
        self.assertIn('r.GET("/api/access-profiles/{id}"', ws_go)
        self.assertIn('r.PUT("/api/access-profiles/{id}"', ws_go)
        self.assertIn('r.DELETE("/api/access-profiles/{id}"', ws_go)
        self.assertIn('r.POST("/api/access-profiles/{id}/activate"', ws_go)

        # 3. UI route
        sidebar_ts = (UI_DIR / "components" / "sidebar.tsx").read_text(encoding="utf-8")
        self.assertIn('/workspace/governance/access-profiles', sidebar_ts)

    # -------------------------------------------------------------------------
    # 9. Audit Logs Connection
    # -------------------------------------------------------------------------
    def test_09_audit_logs_connection(self):
        """Verify Audit Logs DB table, export/settings handlers, and UI views."""
        # 1. DB table & logging activity
        self.cur.execute("SELECT count(*) FROM audit_logs")
        self.assertGreater(self.cur.fetchone()["count"], 3000)

        # 2. Backend routes
        ws_go = (HANDLERS_DIR / "workspace.go").read_text(encoding="utf-8")
        self.assertIn('r.GET("/api/audit-logs"', ws_go)
        self.assertIn('r.GET("/api/audit-logs/export"', ws_go)
        self.assertIn('r.GET("/api/audit-logs/settings"', ws_go)
        self.assertIn('r.PUT("/api/audit-logs/settings"', ws_go)

        # 3. UI route
        sidebar_ts = (UI_DIR / "components" / "sidebar.tsx").read_text(encoding="utf-8")
        self.assertIn('/workspace/audit-logs', sidebar_ts)


if __name__ == "__main__":
    unittest.main()
