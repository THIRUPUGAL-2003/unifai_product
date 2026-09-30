import { Navigate, createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/workspace/providers/model-limits")({
	component: () => <Navigate to="/workspace/model-limits" replace />,
});
