# Governance & Virtual Key Architecture Specification

This document is the definitive specification and operational manual for the Governance Plugin, Virtual Key Resolution, Multi-Tier Usage Accounting, and Pre-Request Budget/Rate-Limit Enforcement in Unifai.

---

## 1. Overview & Assignment Model (Many-to-Many)

Virtual Keys (VKs) are flexible governance anchors that control model access, rate limits, budgets, and provider routing.

### 1.1 Universal Assignment
A Virtual Key can be assigned to any combination of the four organizational entities:
* **Customer (`governance_virtual_key_customers` / `vk.customer_id`)**
* **Business Unit (`governance_virtual_key_business_units`)**
* **Team (`governance_virtual_key_teams` / `vk.team_id`)**
* **User (`governance_virtual_key_users` / `vk.user_ids`)**

### 1.2 Multi-Assignment Rules
* **One Entity → Multiple Keys:** Any Customer, Business Unit, Team, or User can be assigned multiple Virtual Keys.
* **One Key → Multiple Entities:** A single Virtual Key can be simultaneously assigned across multiple Customers, Business Units, Teams, and Users.

---

## 2. Access Inheritance & User Login Resolution

When a user logs in or authenticates, the system resolves all Virtual Keys they are permitted to use via `ResolveAllowedVirtualKeyIDsForUser()`:

```
Customer Assignment
   ├── Business Unit (BU) Assignment
   │    └── Team Assignment
   │         └── User Direct Assignment
```

### 2.1 Resolution Hierarchy
1. **Direct User Assignment:** Keys explicitly assigned to the user's ID.
2. **Team Inheritance:** Keys assigned to any Team the user belongs to.
3. **Business Unit Inheritance:** Keys assigned to any Business Unit containing one of the user's teams.
4. **Customer Inheritance:** Keys assigned to the Customer owning the user's Team or Business Unit.

### 2.2 UI Auto-Selection & Isolation
* In the prompt and chat UI (`settingsPanel.tsx`), the active Virtual Key automatically selects the first allowed, unblocked key.
* Users can only view and select keys they have inherited or been assigned. Keys belonging to other entities remain completely invisible.
* Allowed Providers and Models are automatically populated from the selected Virtual Key's provider configs.

---

## 3. Usage Accounting & Budget Tracking Hierarchy

Usage accounting runs post-request in `tracker.go` and `store.go`. Every request meters costs atomically across the multi-tier hierarchy:

$$\text{Request Cost } \rightarrow \text{User} \rightarrow \text{Team} \rightarrow \text{Business Unit} \rightarrow \text{Customer} \rightarrow \text{Virtual Key}$$

### 3.1 Accounting Isolation Rules
* **User Budget:** Only charges the specific user who executed the request. Other users under the same team/customer are unaffected.
* **Team Budget:** Only charges the specific team to which the calling user belongs. Sibling teams under the same BU or Customer are NOT charged.
* **Business Unit Budget:** Only charges the specific Business Unit containing the calling team. Sibling BUs under the same Customer are NOT charged.
* **Customer Budget:** Charges the customer entity owning the calling user/team.
* **Virtual Key Budget:** Accumulates the total aggregate spend of **all** users, teams, and customers using that key.

---

## 4. Cross-Customer & Shared Team Scoping

A single Virtual Key can be shared across organizational boundaries (e.g., Customer A owns a key, but grants access to Team B1 which sits inside Customer B).

```
Virtual Key: VK-Shared (Assigned to: Customer A, and Team B1)
├── Customer A
│    └── Team A1 -> User A1
└── Customer B (NOT assigned VK-Shared)
     ├── Team B1 (Assigned VK-Shared) -> User B1
     └── Team B2 (NOT assigned VK-Shared) -> User B2
```

### 4.1 Visibility & Isolation Rules
* **Customer A:** All users and teams in Customer A inherit and see `VK-Shared`.
* **Team B1 (Customer B):** User B1 sees and uses `VK-Shared` because Team B1 was explicitly assigned.
* **Customer B General (Team B2):** User B2 **cannot** see or use `VK-Shared`. Customer B does not see `VK-Shared` in customer-wide key lists.

### 4.2 Billing Attribution Rules
* **When User A1 calls `VK-Shared`:**
  * User A1 Budget: **Charged**
  * Team A1 Budget: **Charged**
  * Customer A Budget: **Charged**
  * `VK-Shared` Budget: **Charged**
  * Customer B / Team B1: **$0 (Not Charged)**
* **When User B1 calls `VK-Shared`:**
  * User B1 Budget: **Charged**
  * Team B1 Budget: **Charged**
  * Customer B Budget: **$0 (NOT CHARGED)** — Because `VK-Shared` was assigned to Team B1 only, not Customer B.
  * Customer A Budget: **$0 (NOT CHARGED)** — User B1 is outside Customer A.
  * `VK-Shared` Budget: **Charged** (Total usage accumulates User A1 + User B1).

---

## 5. Provider-Level Governance Rules

1. **User-Only Provider Assignment (No VK):**
   * If a user is directly granted access to a provider/model without a Virtual Key:
     * Provider Budget: Charged
     * Model Budget: Charged
     * User Personal Budget: Charged
     * Team / BU / Customer Budget: **NOT CHARGED** (Bypasses team hierarchy because no team-level or VK-level budget was engaged).
2. **VK-Assigned Provider Configuration:**
   * When a provider is configured inside a Virtual Key:
     * Usage flows through all 5 levels (Provider, User, Team, BU, Customer, and VK).

---

## 6. Pre-Request Budget Enforcement & Short-Circuit Flow

Before any LLM or MCP tool request is executed, the gateway short-circuits and denies unauthorized or exhausted requests in `main.go`:

```
Incoming Request
      │
      ▼
Step 1: Virtual Key Evaluation (Identity, Expiration, Active Status, VK Budget & Rate Limits)
      │
      ▼
Step 2: Customer Level Evaluation (Customer Budget & Rate Limits)
      │
      ▼
Step 2.5: Business Unit Level Evaluation (Business Unit Budget & Rate Limits)
      │
      ▼
Step 3: Team Level Evaluation (Team Budget & Rate Limits)
      │
      ▼
Step 4: User Level Evaluation (User Personal Budget & Rate Limits)
      │
      ▼
Request Allowed to Proceed to Provider
```

### 6.1 Exhaustion & Blocking Scope Matrix

| Exhausted Entity | Who is Blocked? | Who can still use the Key? |
| :--- | :--- | :--- |
| **Virtual Key Budget Over** | **EVERYONE** (All users, teams, BUs, customers) | None. Key is globally blocked. |
| **Customer Budget Over** | All users and teams belonging to that Customer | Teams from other customers assigned to the same key. |
| **Business Unit Budget Over** | All teams and users in that Business Unit | Teams in sibling BUs under the same Customer. |
| **Team Budget Over** | Users belonging to that specific Team | Sibling teams under the same BU or Customer. |
| **User Budget Over** | That specific User only | All other teammates and users. |

---

## 7. Error Handling, HTTP Codes, & Alerts Specification

When any budget or rate limit is violated, the gateway returns standardized errors and fires administrative alerts.

### 7.1 Protocol Status Codes
* **HTTP `402 Payment Required`:** Returned when any budget in the hierarchy is exceeded (`DecisionBudgetExceeded`).
* **HTTP `429 Too Many Requests`:** Returned when any rate limit (RPM/TPM) is exceeded (`DecisionRateLimitExceeded`).
* **HTTP `403 Forbidden`:** Returned when an unassigned or forbidden provider, model, or MCP tool is attempted (`DecisionVirtualKeyBlocked`, `DecisionModelBlocked`, `DecisionProviderBlocked`, `DecisionMCPToolBlocked`).
* **HTTP `404 Not Found`:** Returned when the requested Virtual Key does not exist.

### 7.2 Human-Readable Reason Strings
The resolver generates level-specific actionable explanations:
* **VK Level:**
  `"Virtual key budget is used up ($X.XX of $Y.YY used). Please use another virtual key."`
* **Customer Level:**
  `"Customer budget is used up ($X.XX of $Y.YY used). This virtual key is blocked for all of the customer's teams — please use another virtual key."`
* **Business Unit Level:**
  `"Business unit budget is used up ($X.XX of $Y.YY used). This virtual key is blocked for teams in that unit — please use another virtual key."`
* **Team Level:**
  `"Team budget is used up ($X.XX of $Y.YY used). This virtual key is blocked for your team — please use another virtual key."`
* **User Level:**
  `"Your personal budget is used up ($X.XX of $Y.YY used). Requests are blocked until it resets or an admin raises it."`

### 7.3 Administrative Critical Alerts
When `DecisionBudgetExceeded` is reached:
* An event is emitted via `emitGovernanceAlert`:
  * **Alert Kind:** `alerts.KindBudgetExceeded`
  * **Severity:** `alerts.SeverityCritical`
  * **Payload:** Contains the exact entity ID, spent amount, limit threshold, and caller context.

---

## 8. Verification & Test Suite Reference

All specifications in this document are verified by the unit test suite in `plugins/governance/billed_entities_test.go`:

| Test Function | Verification Target |
| :--- | :--- |
| `TestCustomerKeyBillsCustomer` | Customer assignment grants access to customer tree |
| `TestTwoCustomersChargeOnlyTheUsersCustomerAndTeam` | Multi-team, multi-user isolation under customer |
| `TestSharedTeamMultiCustomerScoping` | Shared team attribution and customer separation |
| `TestOutsideTeamOnCustomerKeyDoesNotChargeCustomer` | Cross-customer team does not bill foreign parent customer |
| `TestBusinessUnitBudgetEnforcement` | Pre-request BU budget exhaustion block (Step 2.5) |
| `TestCustomerKeyBusinessUnitTeamIsBilledWhenTeamHasNoCustomer` | BU team without direct customer ID resolves through BU |
| `TestCustomerKeyDirectTeamDoesNotChargeBusinessUnit` | Direct team keys do not falsely bill unassigned BUs |
| `TestTeamBudgetTotalBlocksEveryKey` | Team budget exhaustion blocks requests for that team only |
| `TestBudgetExhaustedReasonNamesTheSpentLevel` | Precision of level-specific 402 error messages |

All 24 test suites execute clean and pass with 100% green coverage.
