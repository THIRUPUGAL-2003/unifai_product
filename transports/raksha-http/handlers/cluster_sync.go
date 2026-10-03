package handlers

import (
	"io"

	"github.com/raksha/raksha/framework/cluster"
	"github.com/valyala/fasthttp"
)

func (h *WorkspaceHandler) clusterKVReplicate(ctx *fasthttp.RequestCtx) {
	if !cluster.IsReplicationRequest(string(ctx.Request.Header.Peek(cluster.ReplicateHeaderName()))) {
		SendError(ctx, fasthttp.StatusForbidden, "missing cluster replication header")
		return
	}
	provided := string(ctx.Request.Header.Peek(cluster.ReplicateSecretHeaderName()))
	if !cluster.VerifyReplicateSecret(provided) {
		SendError(ctx, fasthttp.StatusUnauthorized, "invalid or missing cluster replication secret — set CLUSTER_REPLICATE_SECRET on all peers")
		return
	}
	if h.store == nil || h.store.KVStore == nil {
		SendError(ctx, fasthttp.StatusServiceUnavailable, "kv store is not available")
		return
	}
	body := ctx.PostBody()
	if len(body) == 0 && ctx.Request.BodyStream() != nil {
		var err error
		body, err = io.ReadAll(ctx.Request.BodyStream())
		if err != nil {
			SendError(ctx, fasthttp.StatusBadRequest, "invalid replication payload")
			return
		}
	}
	msg, err := cluster.DecodeReplicationMessage(body)
	if err != nil {
		SendError(ctx, fasthttp.StatusBadRequest, "invalid replication payload")
		return
	}
	if err := cluster.ApplyReplication(h.store.KVStore, msg); err != nil {
		SendError(ctx, fasthttp.StatusInternalServerError, "failed to apply replication")
		return
	}
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}
