import { Navigate, createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/workspace/providers/routing-rules")({
	component: () => <Navigate to="/workspace/routing-rules" replace />,
});
