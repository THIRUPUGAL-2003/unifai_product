import { Button } from "@/components/ui/button";
import { getApiBaseUrl } from "@/lib/utils/port";
import { Link } from "@tanstack/react-router";
import { CheckCircle2, Loader2, XCircle } from "lucide-react";
import { useEffect, useState } from "react";

type CallbackStatus = "loading" | "success" | "error";

export default function DiscoverCallbackView() {
	const [status, setStatus] = useState<CallbackStatus>("loading");
	const [message, setMessage] = useState("Completing SCIM OAuth discovery…");

	useEffect(() => {
		const params = new URLSearchParams(window.location.search);
		const code = params.get("code");
		const state = params.get("state");
		const error = params.get("error");
		const errorDescription = params.get("error_description");

		const notifyOpener = (type: "scim_oauth_success" | "scim_oauth_failed", detail?: string) => {
			if (window.opener) {
				window.opener.postMessage({ type, error: detail }, window.location.origin);
				window.close();
			}
		};

		if (error) {
			const detail = errorDescription || error;
			setStatus("error");
			setMessage(detail);
			notifyOpener("scim_oauth_failed", detail);
			return;
		}

		if (!code) {
			setStatus("error");
			setMessage("Missing authorization code in callback URL.");
			notifyOpener("scim_oauth_failed", "Missing authorization code");
			return;
		}

		const complete = async () => {
			try {
				const res = await fetch(`${getApiBaseUrl()}/scim/oauth/callback`, {
					method: "POST",
					credentials: "include",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({ code, state }),
				});
				if (!res.ok) {
					const body = await res.json().catch(() => ({}));
					const detail = body?.error?.message || body?.message || `OAuth callback failed (${res.status})`;
					setStatus("error");
					setMessage(detail);
					notifyOpener("scim_oauth_failed", detail);
					return;
				}
				setStatus("success");
				setMessage("SCIM OAuth discovery completed. You can return to SCIM settings.");
				notifyOpener("scim_oauth_success");
			} catch (err) {
				const detail = err instanceof Error ? err.message : "Network error during OAuth callback";
				setStatus("error");
				setMessage(detail);
				notifyOpener("scim_oauth_failed", detail);
			}
		};

		void complete();
	}, []);

	return (
		<div className="mx-auto flex min-h-[60vh] w-full max-w-xl items-center justify-center p-6">
			<div className="bg-card w-full rounded-lg border p-8 text-center shadow-sm">
				{status === "loading" && <Loader2 className="text-muted-foreground mx-auto mb-4 h-10 w-10 animate-spin" />}
				{status === "success" && <CheckCircle2 className="mx-auto mb-4 h-10 w-10 text-emerald-500" />}
				{status === "error" && <XCircle className="text-destructive mx-auto mb-4 h-10 w-10" />}
				<h1 className="text-xl font-semibold">
					{status === "loading" ? "Processing OAuth callback" : status === "success" ? "Discovery complete" : "Discovery failed"}
				</h1>
				<p className="text-muted-foreground mt-3 text-sm">{message}</p>
				<div className="mt-6">
					<Button asChild variant="outline">
						<Link to="/workspace/scim">Back to SCIM settings</Link>
					</Button>
				</div>
			</div>
		</div>
	);
}
