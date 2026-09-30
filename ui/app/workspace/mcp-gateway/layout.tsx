import { createFileRoute } from "@tanstack/react-router";
import MCPGatewayRedirectPage from "./page";

export const Route = createFileRoute("/workspace/mcp-gateway")({
	component: MCPGatewayRedirectPage,
});
