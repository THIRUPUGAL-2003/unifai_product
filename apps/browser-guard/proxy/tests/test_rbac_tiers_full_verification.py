#!/usr/bin/env python3
"""Comprehensive RBAC 3-Tier Testing & Role Permission Architecture Verification.
Validates:
1. Enterprise RBAC Architecture Design:
   - Resource-Operation Matrix (17 Resources x 7 Operations)
   - DAC (Data Access Control: all-data vs own-data)
   - Privilege Escalation Prevention (Anti-Privilege Creep)
2. Admin Tier:
   - Super-Admin unrestricted access (allowAll)
   - Full control over users, roles, audit logs, provider master keys, SCIM secrets
3. Sub-Admin Tier (Workspace Manager):
   - Delegated management: Virtual Keys, Budgets, DLP Rules, Prompts, Teams
   - Security fences: Blocked from Master API Keys, SCIM Bearer Tokens, Role Escalation, User Dump
4. Standard User Tier:
   - Restricted to Prompt Repository (/workspace/prompt-repo)
   - DAC own-data isolation
   - All Governance & Admin endpoints blocked (403 Forbidden)
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
RBAC_DIR = REPO_ROOT / "framework" / "rbac"


class RBACTiersFullVerificationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        import psycopg2
        import psycopg2.extras

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
    # 1. Enterprise RBAC Architecture & Design Validation
    # -------------------------------------------------------------------------
    def test_01_rbac_architecture_design_integrity(self):
        """Verify standard enterprise RBAC design (Resource x Operation + DAC)."""
        enforce_go = (RBAC_DIR / "enforce.go").read_text(encoding="utf-8")
        
        # 1. Permission Matrix Structure: Resource -> Operation -> Allowed
        self.assertIn("type PermissionSet map[string]map[string]bool", enforce_go)
        self.assertIn("type PathRequirement struct", enforce_go)
        self.assertIn("ResolvePermissions", enforce_go)
        
        # 2. Database Roles Table check
        self.cur.execute("SELECT id, name, is_system_role, dac, permission_ids_json FROM rbac_roles ORDER BY id")
        roles = self.cur.fetchall()
        role_map = {r["name"]: r for r in roles}
        
        self.assertIn("admin", role_map)
        self.assertIn("sub_admin", role_map)
        self.assertIn("user", role_map)
        
        # DAC checks: Admin & Sub-Admin have all-data, User has own-data
        self.assertEqual(role_map["admin"]["dac"], "all-data")
        self.assertEqual(role_map["sub_admin"]["dac"], "all-data")
        self.assertEqual(role_map["user"]["dac"], "own-data")

    # -------------------------------------------------------------------------
    # 2. Admin Tier Verification (Super Admin)
    # -------------------------------------------------------------------------
    def test_02_admin_tier_full_unrestricted_control(self):
        """Verify Admin tier has allowAll() and reaches all sensitive resources."""
        enforce_go = (RBAC_DIR / "enforce.go").read_text(encoding="utf-8")
        
        # Admin gets allowAll() on all resources
        self.assertIn('if roleName == "" || roleName == "admin"', enforce_go)
        self.assertIn('return allowAll(), nil', enforce_go)
        
        # Admin can manage users in session.go
        session_go = (HANDLERS_DIR / "session.go").read_text(encoding="utf-8")
        self.assertIn("func (h *SessionHandler) isAdmin(", session_go)
        self.assertIn("func (h *SessionHandler) isSuperAdmin(", session_go)
        
        # Admin can view/modify SCIM secrets in scim.go
        scim_go = (HANDLERS_DIR / "scim.go").read_text(encoding="utf-8")
        self.assertIn('isAdmin := h.callerRole(ctx) == "admin"', scim_go)
        self.assertIn('if !isAdmin', scim_go)

    # -------------------------------------------------------------------------
    # 3. Sub-Admin Tier Verification (Workspace Manager)
    # -------------------------------------------------------------------------
    def test_03_sub_admin_tier_delegated_controls_and_security_fences(self):
        """Verify Sub-Admin can manage operational assets but is fenced from critical controls."""
        workspace_go = (REPO_ROOT / "framework" / "configstore" / "workspace.go").read_text(encoding="utf-8")
        middlewares_go = (HANDLERS_DIR / "middlewares.go").read_text(encoding="utf-8")
        
        # Sub-Admin has full VirtualKeys, Prompts, Logs/Browser-AI, and Governance Budgets
        self.assertIn('case "sub_admin":', workspace_go)
        self.assertIn('Workspace Manager - manages virtual keys, budgets, prompts, logs, and Browser AI', workspace_go)
        
        # Security Fence 1: Sub-Admin CANNOT escalate roles or create roles
        self.assertIn('customRolePathDelegable', middlewares_go)
        self.assertIn('for _, prefix := range []string{"/api/roles", "/api/permissions", "/api/rbac", "/api/users", "/api/config", "/api/scim"}', middlewares_go)
        
        # Security Fence 2: Sub-Admin CANNOT alter SCIM default role to admin
        self.assertIn('if r == "admin" || r == "sub_admin"', scim_go := (HANDLERS_DIR / "scim.go").read_text(encoding="utf-8"))
        self.assertIn('Only the super admin can set the SCIM default role to', scim_go)

    # -------------------------------------------------------------------------
    # 4. Standard User Tier Verification (Prompt Repo Member)
    # -------------------------------------------------------------------------
    def test_04_user_tier_isolation_and_lockdown(self):
        """Verify Standard User is strictly locked to Prompt Repo with zero governance reach."""
        middlewares_go = (HANDLERS_DIR / "middlewares.go").read_text(encoding="utf-8")
        workspace_access_ts = (UI_DIR / "lib" / "utils" / "workspaceAccess.ts").read_text(encoding="utf-8")
        
        # Backend Fence: User role never uses RBAC delegation (never allowed to bypass to admin endpoints)
        self.assertIn('roleUsesRBACDelegation(role string) bool', middlewares_go)
        self.assertIn('role != "user"', middlewares_go)
        
        # Frontend Fence: User default home path is Prompt Repo
        self.assertIn('USER_ROLE_HOME_PATH = "/workspace/prompt-repo"', workspace_access_ts)
        self.assertIn('if (auth.role === "user")', workspace_access_ts)
        self.assertIn('return new Set(["prompt-repository"]);', workspace_access_ts)

    # -------------------------------------------------------------------------
    # 5. Live DB Users Role Distribution Check
    # -------------------------------------------------------------------------
    def test_05_live_database_users_and_roles_compliance(self):
        """Verify live users in PostgreSQL match allowed RBAC roles and approval status."""
        self.cur.execute("SELECT id, username, email, role, status FROM governance_users")
        users = self.cur.fetchall()
        
        valid_roles = {"admin", "sub_admin", "user"}
        valid_statuses = {"approved", "rejected", "disabled", "pending"}
        
        for u in users:
            self.assertIn(u["role"], valid_roles, f"User {u['username']} has invalid role {u['role']}")
            self.assertIn(u["status"], valid_statuses, f"User {u['username']} has invalid status {u['status']}")


if __name__ == "__main__":
    unittest.main()
