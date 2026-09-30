import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";

/** Legacy / bookmark URL — MCP Gateway home is MCP Catalog. */
export default function MCPGatewayRedirectPage() {
	const navigate = useNavigate();
	useEffect(() => {
		navigate({ to: "/workspace/mcp-registry", replace: true });
	}, [navigate]);
	return null;
}
