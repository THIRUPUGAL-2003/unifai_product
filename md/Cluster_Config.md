# UnifAI Cluster Configuration & High Availability Architecture — Complete Reference Guide

This document is the **definitive, production-grade guide** for configuring, operating, and troubleshooting **Multi-Node High Availability Clusters** (`/workspace/cluster`) in the UnifAI Gateway.

---

## Table of Contents

1. [Cluster Overview & High Availability Architecture](#1-cluster-overview--high-availability-architecture)
2. [Topology Models: Mesh vs. Broker](#2-topology-models-mesh-vs-broker)
3. [Configuration Parameters & Schema](#3-configuration-parameters--schema)
4. [Step-by-Step Setup & How to Connect Nodes](#4-step-by-step-setup--how-to-connect-nodes)
5. [Real-Time State Replication (`KVStore` & Secret Auth)](#5-real-time-state-replication-kvstore--secret-auth)
6. [Adaptive Load Balancing & Auto-Failover Integration](#6-adaptive-load-balancing--auto-failover-integration)
7. [API Endpoints & Practical cURL Payloads](#7-api-endpoints--practical-curl-payloads)
8. [Exhaustive Error Handling & Troubleshooting Guide](#8-exhaustive-error-handling--troubleshooting-guide)
9. [Pre-Flight Verification & Readiness Checklist](#9-pre-flight-verification--readiness-checklist)
10. [SOC-2 Audit Trail & Governance Compliance](#10-soc-2-audit-trail--governance-compliance)

---

## 1. Cluster Overview & High Availability Architecture

In a production enterprise deployment, relying on a single gateway instance poses significant risks:
- **Single Point of Failure (SPOF)**: If the host crashes, reboots, or suffers network failure, all AI services go down.
- **Resource Saturation**: High-volume LLM traffic (hundreds or thousands of requests per second) can saturate a single node's CPU, socket buffers, and network interfaces.

**UnifAI Cluster Mode** allows you to run multiple Gateway nodes behind a load balancer (such as AWS ALB, Cloudflare, NGINX, HAProxy, or Kubernetes Ingress) as a unified, coordinated fleet.

```mermaid
flowchart TD
    ClientTraffic[Enterprise Client Applications] --> LoadBalancer[Layer 7 Load Balancer: AWS ALB / NGINX / K8s]
    LoadBalancer -->|Route Request| Node1[Gateway Node 1: 10.0.0.11:6000]
    LoadBalancer -->|Route Request| Node2[Gateway Node 2: 10.0.0.12:6000]
    LoadBalancer -->|Route Request| Node3[Gateway Node 3: 10.0.0.13:6000]

    Node1 <==>|Peer Replication /api/cluster/replicate| Node2
    Node2 <==>|Peer Replication /api/cluster/replicate| Node3
    Node3 <==>|Peer Replication /api/cluster/replicate| Node1

    Node1 & Node2 & Node3 --> SharedDB[(Shared PostgreSQL Database)]
```

### What is Synchronized Across Nodes?
When a request is processed by Node 1, the following runtime state is **instantly replicated** to Node 2 and Node 3:
1. **Virtual Key Token Budgets**: Prevents a customer or team from exceeding their allocated monthly budget across parallel requests.
2. **Rate Limit Counters (RPM / TPM)**: Ensures rate limits are strictly enforced globally, not per-node.
3. **Circuit Breaker Health Scores**: If an upstream LLM provider fails on Node 1, all nodes immediately route traffic to healthy fallback models.
4. **Session States & Role Changes**: User deactivations or role updates are applied everywhere in real time.

---

## 2. Topology Models: Mesh vs. Broker

| Feature | Mesh Topology (`mesh`) | Broker Topology (`broker`) |
| :--- | :--- | :--- |
| **Architecture** | Peer-to-Peer (Decentralized) | Centralized Hub & Spoke |
| **Best For** | 2 to 10 gateway nodes (most common) | 10+ nodes or complex multi-VPC hubs |
| **Communication** | Direct HTTP replication calls to every peer | Nodes replicate to a central coordinator |
| **Fault Tolerance** | Any single node can fail without affecting others | Redundant brokers required for HA |
| **Default in UnifAI**| **Yes (`type: "mesh"`)** | Optional enterprise configuration |

---

## 3. Configuration Parameters & Schema

Stored in the `workspace_settings` table under key `cluster`.

```json
{
  "enabled": true,
  "type": "mesh",
  "region": "ap-south-1",
  "peers": [
    "http://10.0.0.12:6000",
    "http://10.0.0.13:6000"
  ],
  "gossip": {
    "port": 7946,
    "config": {
      "timeout_seconds": 5,
      "success_threshold": 2,
      "failure_threshold": 3
    }
  },
  "grpc": {
    "port": 50051,
    "dial_timeout_seconds": 10
  }
}
```

### Parameter Reference

| Field | Type | Default | Required? | Description |
| :--- | :--- | :--- | :--- | :--- |
| `enabled` | `boolean` | `false` | **Yes** | Master switch. `false` = Standalone mode; `true` = Multi-node Cluster mode. |
| `type` | `string` | `"mesh"` | **Yes** | Topology: `"mesh"` (peer-to-peer) or `"broker"` (hub-and-spoke). |
| `region` | `string` | `"unknown"` | No | Cloud / Data Center region (e.g. `ap-south-1`, `us-east-1`, `eu-central-1`). |
| `peers` | `string[]` | `[]` | **Yes (if enabled)**| List of peer gateway addresses. Must contain **at least one** peer when enabled. |
| `gossip.port` | `integer` | `7946` | No | Internal UDP/TCP gossip discovery port. |
| `gossip.config.timeout_seconds` | `integer` | `5` | No | Heartbeat timeout between cluster nodes. |
| `gossip.config.success_threshold`| `integer` | `2` | No | Consecutive healthy pings before marking a node online. |
| `gossip.config.failure_threshold`| `integer` | `3` | No | Consecutive failed pings before evicting a node. |
| `grpc.port` | `integer` | `50051` | No | High-speed gRPC replication port. |

---

## 4. Step-by-Step Setup & How to Connect Nodes

### Example Scenario: Setting Up a 2-Node Cluster
* **Node 1 Internal IP**: `10.0.0.11`, listening on port `6000`.
* **Node 2 Internal IP**: `10.0.0.12`, listening on port `6000`.

---

### Step 1: Set the Same Shared Secret on Both Nodes
In the `.env` file of **both** Node 1 and Node 2, set the exact same replication secret:
```env
# Multi-node KV replication (must match identically on all cluster nodes)
CLUSTER_REPLICATE_SECRET=unifai-prod-cluster-secret-key-32bytes-secure-2026
```

### Step 2: Connect Both Nodes to the Same Database
In the `.env` of both nodes, point to your production PostgreSQL database:
```env
DB_TYPE=postgres
DB_HOST=postgres.internal.company.com
DB_PORT=5432
DB_NAME=unifai_production
DB_USER=unifai_admin
DB_PASSWORD=SecurePassword#2026
DB_SSL_MODE=require
```

### Step 3: Configure Peers on Node 1
1. Open the UnifAI Dashboard on Node 1: `http://10.0.0.11:3100/workspace/cluster`.
2. Under **Peer Nodes**, enter Node 2's address:
   ```text
   http://10.0.0.12:6000
   ```
3. Toggle **Cluster Mode: ON**.
4. Click **Save Changes**.

### Step 4: Configure Peers on Node 2
1. Open the UnifAI Dashboard on Node 2: `http://10.0.0.12:3100/workspace/cluster`.
2. Under **Peer Nodes**, enter Node 1's address:
   ```text
   http://10.0.0.11:6000
   ```
3. Toggle **Cluster Mode: ON**.
4. Click **Save Changes**.

### Step 5: Verification
1. On both nodes, verify the Node status badge displays:
   - **Mode**: `Cluster` (Green badge).
   - **Replication Secret**: `Configured` (Green checkmark).
   - **Warnings**: `0 Warnings`.

---

## 5. Real-Time State Replication (`KVStore` & Secret Auth)

When a node mutates state (e.g. key budget deduction), it calls the internal replication endpoint on all peer nodes:
```http
POST /api/cluster/replicate HTTP/1.1
Host: 10.0.0.12:6000
X-Gateway-Replicate: true
X-Gateway-Replicate-Secret: <CLUSTER_REPLICATE_SECRET>
Content-Type: application/json

{
  "key": "budget:user:9a8b7c",
  "operation": "SET",
  "value": "45.20",
  "version": 104
}
```

### Security & Authentication Protocol
1. **Header Check**: `X-Gateway-Replicate: true` must be present. If missing, returns `403 Forbidden`.
2. **HMAC/Constant-Time Secret Verification**: `X-Gateway-Replicate-Secret` must match `CLUSTER_REPLICATE_SECRET`. If mismatched, returns `401 Unauthorized`.
3. **Internal Only**: External client requests without this header are strictly blocked.

---

## 6. Adaptive Load Balancing & Auto-Failover Integration

Cluster mode integrates seamlessly with the **Adaptive Routing & Load Balancing** subsystem:

```json
{
  "enabled": true,
  "direction_selection_enabled": true,
  "route_selection_enabled": true,
  "reroute_failed_directions": true,
  "prune_failed_fallbacks": true
}
```

* **`reroute_failed_directions`**: If Node 1 detects that an upstream provider or peer node has become unresponsive, traffic is automatically rerouted to healthy nodes and healthy provider endpoints without throwing 500 errors to clients.
* **`prune_failed_fallbacks`**: Unhealthy nodes and degraded routes are automatically dropped from the active fallback cascade until they recover.

---

## 7. API Endpoints & Practical cURL Payloads

### 7.1 Inspect Current Cluster Status
```bash
curl --location 'http://localhost:6000/api/cluster' \
  --cookie cookies.txt
```
**Response:**
```json
{
  "enabled": true,
  "type": "mesh",
  "region": "ap-south-1",
  "peers": [
    "http://10.0.0.12:6000"
  ],
  "node": {
    "address": "10.0.0.11:6000",
    "mode": "cluster"
  },
  "replicate_secret_configured": true,
  "warnings": []
}
```

### 7.2 Save Cluster Configuration
```bash
curl --location --request PUT 'http://localhost:6000/api/cluster' \
  --header 'Content-Type: application/json' \
  --cookie cookies.txt \
  --data '{
    "enabled": true,
    "type": "mesh",
    "region": "ap-south-1",
    "peers": [
      "http://10.0.0.12:6000",
      "http://10.0.0.13:6000"
    ]
  }'
```

### 7.3 Manual Peer Replication Test
```bash
curl --location 'http://10.0.0.12:6000/api/cluster/replicate' \
  --header 'X-Gateway-Replicate: true' \
  --header 'X-Gateway-Replicate-Secret: unifai-prod-cluster-secret-key-32bytes-secure-2026' \
  --header 'Content-Type: application/json' \
  --data '{
    "key": "test:ping",
    "operation": "SET",
    "value": "ok"
  }'
```
*Expected Response:* `204 No Content` (Success).

---

## 8. Exhaustive Error Handling & Troubleshooting Guide

Every validation rule, error message, root cause, and step-by-step fix:

| Error Message | Exact Root Cause | Step-by-Step Fix |
| :--- | :--- | :--- |
| `add at least one peer when cluster mode is enabled` | Cluster Mode toggle was set to **Enabled**, but the **Peers** list was left empty. | Enter at least one peer gateway address (e.g. `http://10.0.0.12:6000`) before enabling cluster mode. |
| `type must be mesh or broker` | Payload had an unsupported cluster topology. | Set `"type": "mesh"` or `"type": "broker"`. |
| `Invalid peer "..." — only http:// or https:// URLs are supported` | Peer address included an unsupported protocol (e.g. `ftp://`, `tcp://`, or `grpc://`). | Use standard HTTP/HTTPS: `http://10.0.0.12:6000`. |
| `Invalid peer "..." — give only the gateway address, without a path` | Peer address included a URL path (e.g. `http://10.0.0.12:6000/api/v1`). | Remove the path. Provide only the host and port: `http://10.0.0.12:6000`. |
| `Invalid peer "..." — do not include a username or password` | Peer address contained credentials (e.g. `http://user:pass@10.0.0.12:6000`). | Remove credentials. Cluster authentication uses `CLUSTER_REPLICATE_SECRET`, not HTTP basic auth. |
| `Invalid peer "..." — port is required (e.g. 10.0.0.12:6000)` | Peer address was given without a port (e.g. `10.0.0.12`). | Include the listening port: `10.0.0.12:6000` or `http://10.0.0.12:6000`. |
| `Warning: Peer "..." is this node; list only the other nodes.` | The node's own IP/hostname was added to its own peers list. | Remove the current node from the peers list. List **only other** cluster nodes. |
| `Warning: Peer "..." uses the gossip port. Replication calls the gateway HTTP address` | Port `7946` was specified instead of the Gateway API port. | Change the port to the Gateway HTTP port (`6000`), which handles state replication. |
| `Warning: CLUSTER_REPLICATE_SECRET is not set on this node, so nothing is replicated` | `CLUSTER_REPLICATE_SECRET` is blank in `.env`. | Add `CLUSTER_REPLICATE_SECRET=<your-secret>` to `.env` on all nodes and restart. |
| `401 Unauthorized: invalid or missing cluster replication secret` | Peer sent a mismatched or absent `X-Gateway-Replicate-Secret`. | Ensure all nodes share the **identical** `CLUSTER_REPLICATE_SECRET` value. |
| `403 Forbidden: missing cluster replication header` | Request was sent without `X-Gateway-Replicate: true`. | Ensure internal replication clients send the required header. |
| `503 Service Unavailable: kv store is not available` | In-memory KV Store was not initialized at gateway boot. | Verify `kvstore` is enabled in gateway startup configuration. |

---

## 9. Pre-Flight Verification & Readiness Checklist

Before moving a cluster into production, verify each item:

- [ ] **Shared Database**: All cluster nodes are connected to the identical PostgreSQL database instance.
- [ ] **Replication Secret**: `CLUSTER_REPLICATE_SECRET` is set in `.env` with the same 32+ character passphrase on every node.
- [ ] **Network Ports**: Firewall / Security Groups allow TCP port `6000` (Gateway HTTP) and port `7946` (Gossip) between cluster nodes.
- [ ] **Symmetric Peer Lists**:
  - Node 1 lists Node 2 and Node 3.
  - Node 2 lists Node 1 and Node 3.
  - Node 3 lists Node 1 and Node 2.
- [ ] **Zero Self-References**: No node lists its own address in its peers list.
- [ ] **Health Check URL**: Load balancer is configured with target health check path `/health`.
- [ ] **Dashboard Check**: `/workspace/cluster` shows `Mode: Cluster`, `Replication Secret: Configured`, and `0 Warnings`.

---

## 10. SOC-2 Audit Trail & Governance Compliance

Every cluster mutation is recorded as an immutable event in the `audit_logs` table:

| Action | Target Endpoint | Audit Action | Audit Target Name Recorded |
| :--- | :--- | :--- | :--- |
| Enable / Disable Cluster | `PUT /api/cluster` | `UPDATE` | `Cluster Configuration` |
| Add / Remove Peer Nodes | `PUT /api/cluster` | `UPDATE` | `Cluster Configuration` |
| Update Load Balancer Rules | `PUT /api/cluster/load-balancer` | `UPDATE` | `Load Balancer Configuration` |

Audits record the user ID, client IP, timestamp, and before/after configuration state for enterprise security audits.
