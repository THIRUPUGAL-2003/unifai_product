import { createFileRoute } from "@tanstack/react-router";
import BrowserAiPage from "./page";

function RouteComponent() {
	return <BrowserAiPage />;
}

export const Route = createFileRoute("/workspace/browser-ai")({
	component: RouteComponent,
});
