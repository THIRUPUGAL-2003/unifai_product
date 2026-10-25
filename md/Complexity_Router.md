# UnifAI Complexity Router Architecture & Operations Guide

**Route:** [`/workspace/complexity-router`](http://localhost:3100/workspace/complexity-router)  
**Backend Handler:** [`transports/gateway-http/handlers/governance.go`](file:///d:/unifai_project/transports/gateway-http/handlers/governance.go)  
**UI Component:** [`ui/app/workspace/complexity-router/page.tsx`](file:///d:/unifai_project/ui/app/workspace/complexity-router/page.tsx)  
**Database Table:** `governance_config` (Key: `complexity_analyzer_config`) in PostgreSQL database `Unifai_test`

---

## 1. Overview & Purpose

The **Complexity Router** is an intelligent prompt pre-classifier designed to dramatically reduce enterprise LLM operational costs (often by 70% to 90%) while maintaining top-tier output quality.

Instead of sending every user prompt to an expensive frontier reasoning model (e.g., `o1`, `gpt-4o`, `claude-3-opus`), the Complexity Router evaluates incoming prompts in under **2 milliseconds** using lexical token analysis, syntactic heuristics, and domain keyword density scoring.

Prompts are dynamically classified into discrete tiers:
1. **Simple**: Lightweight queries (e.g., "Summarize this email", "Fix typo", "Translate to Spanish"). Routed to inexpensive fast models (e.g., `gpt-4o-mini`, `claude-3-haiku`, `llama-3-8b`).
2. **Medium**: Standard development, drafting, and conversational queries (e.g., "Write a Python script for CSV parsing"). Routed to standard balanced models (e.g., `gpt-4o`, `claude-3-5-sonnet`).
3. **Complex**: Multi-layered engineering, architectural design, root-cause debugging, or distributed systems questions. Routed to high-capacity frontier models (e.g., `claude-3-5-sonnet`, `gemini-1.5-pro`).
4. **Reasoning**: Formal logic proofs, mathematical formulations, and deep chain-of-thought problems. Routed to frontier reasoning engines (e.g., `o1`, `o3-mini`, `deepseek-r1`).

```mermaid
flowchart TD
    Prompt[Inbound User Prompt] --> Lexer[Fast Lexical Tokenizer]
    Lexer --> KWMatch[Keyword Density Matching: Code / Technical / Reasoning / Simple]
    Lexer --> LengthScore[Prompt Token Length & Structural Complexity]
    KWMatch & LengthScore --> ScoreCalc[Calculate Aggregate Complexity Score: 0.00 to 1.00]
    
    ScoreCalc --> CutoffCheck{Score vs Configured Tier Boundaries}
    CutoffCheck -->|Score < simple_medium (0.15)| TierSimple[Tier: Simple -> gpt-4o-mini / claude-3-haiku]
    CutoffCheck -->|0.15 <= Score < medium_complex (0.35)| TierMedium[Tier: Medium -> gpt-4o / claude-3-5-sonnet]
    CutoffCheck -->|0.35 <= Score < complex_reasoning (0.60)| TierComplex[Tier: Complex -> claude-3-5-sonnet / gemini-1.5-pro]
    CutoffCheck -->|Score >= complex_reasoning (0.60)| TierReasoning[Tier: Reasoning -> o1 / o3-mini / deepseek-r1]

    TierSimple & TierMedium & TierComplex & TierReasoning --> RoutingContext[Inject 'complexity_tier' into CEL Routing Rules Engine]
```

---

## 2. Configuration Parameters

The Complexity Analyzer configuration is stored as a JSON object in PostgreSQL under the `governance_config` table with `key = 'complexity_analyzer_config'`.

### 2.1 Tier Boundaries

| Parameter | Type | Default | Description |
| :--- | :--- | :---: | :--- |
| `tier_boundaries.simple_medium` | `float` | `0.15` | Upper boundary score for Simple queries. Above this is classified as Medium. |
| `tier_boundaries.medium_complex` | `float` | `0.35` | Upper boundary score for Medium queries. Above this is classified as Complex. |
| `tier_boundaries.complex_reasoning` | `float` | `0.60` | Frontier boundary triggering heavy Chain-of-Thought / reasoning models. |

### 2.2 Keyword Categories

| Category Key | Default Item Count | Examples | Description |
| :--- | :---: | :--- | :--- |
| `code_keywords` | 50+ | `algorithm`, `api`, `async`, `class`, `database`, `docker`, `function`, `git`, `interface`, `json`, `kubernetes`, `microservice`, `mutex`, `oauth`, `orm`, `refactor`, `sql`, `thread` | Indicates software development, backend engineering, or algorithmic requests. |
| `technical_keywords` | 60+ | `architecture`, `benchmark`, `cluster`, `concurrency`, `consensus`, `cryptography`, `distributed`, `firewall`, `infrastructure`, `latency`, `load balancer`, `replication`, `scalability`, `soc 2` | Indicates deep system architecture or enterprise infrastructure requirements. |
| `reasoning_keywords` | 15+ | `analyze`, `compare and contrast`, `deduce`, `derive`, `evaluate`, `implications`, `mathematical proof`, `root cause`, `step by step`, `think through`, `tradeoffs`, `why does` | Indicates analytical, logical deduction, or chain-of-thought requirements. |
| `simple_keywords` | 20+ | `brief`, `define`, `format`, `grammar`, `hello`, `hi`, `paraphrase`, `short`, `spell check`, `summarize`, `synonym`, `translate`, `what is`, `who is` | Indicates lightweight, non-intensive conversational queries. |

---

## 3. Database Schema & Storage

- **Table:** `governance_config`
- **Primary Key:** `key` (`VARCHAR`)
- **Value:** `value` (`TEXT` - serialized JSON)

### Inspect Live DB Configuration
```sql
SELECT key, value 
FROM governance_config 
WHERE key = 'complexity_analyzer_config';
```

```json
{
  "tier_boundaries": {
    "simple_medium": 0.15,
    "medium_complex": 0.35,
    "complex_reasoning": 0.60
  },
  "keywords": {
    "code_keywords": ["algorithm", "api", "async", "..."],
    "technical_keywords": ["architecture", "benchmark", "..."],
    "reasoning_keywords": ["analyze", "compare and contrast", "..."],
    "simple_keywords": ["brief", "define", "..."]
  }
}
```

---

## 4. REST API Reference

All requests require authentication with session cookie (`Cookie: token=...`) or bearer token (`Authorization: Bearer <token>`).

### 4.1 Get Complexity Analyzer Configuration
- **Method:** `GET`
- **Path:** `/api/governance/complexity-analyzer-config`
- **Response `200 OK`:**
```json
{
  "tier_boundaries": {
    "simple_medium": 0.15,
    "medium_complex": 0.35,
    "complex_reasoning": 0.60
  },
  "keywords": {
    "code_keywords": ["algorithm", "api", "async", "await", "class", "function"],
    "technical_keywords": ["architecture", "cluster", "concurrency", "distributed"],
    "reasoning_keywords": ["analyze", "evaluate", "root cause", "step by step"],
    "simple_keywords": ["brief", "define", "hello", "short", "summarize"]
  }
}
```

### 4.2 Update Complexity Analyzer Configuration
- **Method:** `PUT`
- **Path:** `/api/governance/complexity-analyzer-config`
- **Payload:**
```json
{
  "tier_boundaries": {
    "simple_medium": 0.18,
    "medium_complex": 0.40,
    "complex_reasoning": 0.65
  },
  "keywords": {
    "code_keywords": ["algorithm", "api", "async", "react", "golang", "rust"],
    "technical_keywords": ["distributed", "kubernetes", "soc 2", "zero trust"],
    "reasoning_keywords": ["step by step", "tradeoffs", "postmortem"],
    "simple_keywords": ["hello", "summarize", "fix typo"]
  }
}
```
- **Response `200 OK`:** Returns updated configuration.

### 4.3 Reset Complexity Analyzer Configuration to Factory Defaults
- **Method:** `POST`
- **Path:** `/api/governance/complexity-analyzer-config/reset`
- **Response `200 OK`:** Restores original built-in tier boundaries and keyword lists.

---

## 5. UI Operations Guide ([`/workspace/complexity-router`](http://localhost:3100/workspace/complexity-router))

1. **Adjust Tier Boundaries**:
   - Navigate to [`/workspace/complexity-router`](http://localhost:3100/workspace/complexity-router).
   - Use the interactive sliders or numeric inputs to adjust `simple_medium`, `medium_complex`, and `complex_reasoning` cutoff points.
2. **Manage Custom Keywords**:
   - In each category tab (**Code**, **Technical**, **Reasoning**, **Simple**), type a new term in the input box and press **Enter** or click **+ Add**.
   - To remove a term, click the **x** badge icon on that keyword tag.
3. **Save Changes**:
   - Click **Save Changes** in the top right corner. The configuration persists instantly to PostgreSQL and reloads the runtime classifier.
4. **Reset to Factory Defaults**:
   - Click **Reset to Defaults** if you ever need to discard customizations and revert to default values.

---

## 6. Connection to CEL Routing Rules Engine

The Complexity Router works synergistically with the **Routing Rules** engine ([`/workspace/routing-rules`](http://localhost:3100/workspace/routing-rules)).

When a prompt is evaluated, the classified tier is exported into the CEL runtime context as the variable `complexity_tier`:
```cel
// Example 1: Route simple queries to low-cost mini model
complexity_tier == "simple"

// Example 2: Route reasoning queries to OpenAI o1
complexity_tier == "reasoning" && request_type == "chat_completion"

// Example 3: Route complex coding queries to Claude 3.5 Sonnet
complexity_tier in ["complex", "reasoning"] && headers["x-team"] == "backend-dev"
```

---

## 7. Troubleshooting & Common Questions

| Question / Symptom | Cause | Solution |
| :--- | :--- | :--- |
| *Queries are classified as "Complex" too frequently* | The `medium_complex` boundary is set too low (e.g. below 0.30) or common team jargon is in `technical_keywords`. | Increase `medium_complex` to `0.40` or remove ambiguous terms from the technical keyword list. |
| *Simple queries are routed to expensive models* | The prompt contains technical buzzwords that elevate its lexical score above `simple_medium`. | Increase `simple_medium` to `0.20` or add recurring phrases to `simple_keywords`. |
| *Reset button clicked by mistake* | `POST /api/governance/complexity-analyzer-config/reset` was triggered. | Re-enter custom proprietary keywords and click Save Changes. |
