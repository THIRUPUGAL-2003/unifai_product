import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { getErrorMessage } from "@/lib/store";
import {
	useGetConnectorQuery,
	useTestConnectorMutation,
	useUpdateConnectorMutation,
} from "@enterprise/lib/store/apis/connectorsApi";
import { Cable, Save } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

interface ConnectorFormProps {
	name: "datadog" | "kafka" | "bigquery" | "pubsub" | "newrelic";
	title: string;
	description: string;
	fields: { key: string; label: string; type?: string; placeholder?: string }[];
	onDelete?: () => void;
	isDeleting?: boolean;
}

export function ConnectorForm({ name, title, description, fields, onDelete, isDeleting }: ConnectorFormProps) {
	const { data } = useGetConnectorQuery(name);
	const [updateConnector, { isLoading: saving }] = useUpdateConnectorMutation();
	const [testConnector, { isLoading: testing }] = useTestConnectorMutation();
	const [enabled, setEnabled] = useState(false);
	const [config, setConfig] = useState<Record<string, string>>({});
	const [lastProbe, setLastProbe] = useState<{ ok?: boolean; error?: string } | null>(null);

	useEffect(() => {
		if (!data) return;
		setEnabled(!!data.enabled);
		setConfig(data.config || {});
		const connection = (data as { connection?: { ok?: boolean; error?: string } }).connection;
		if (connection) setLastProbe(connection);
	}, [data]);

	const save = async () => {
		try {
			const result = await updateConnector({ name, enabled, config }).unwrap();
			const connection = (result as { connection?: { ok?: boolean; error?: string } }).connection;
			if (connection) setLastProbe(connection);
			if (connection && enabled && connection.ok === false) {
				toast.error(connection.error || `${title} saved but connection failed`);
				return;
			}
			if (!enabled) {
				toast.success(`${title} saved (disabled — turn it on to start exporting)`);
			} else if (connection?.ok) {
				toast.success(`${title} connected and saved`);
			} else {
				toast.success(`${title} connector saved`);
			}
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const runTest = async () => {
		try {
			const result = await testConnector(name).unwrap();
			const connection = result.connection;
			if (connection) setLastProbe(connection);
			if (connection?.ok) {
				toast.success(`${title} connection OK`);
			} else {
				toast.error(connection?.error || `${title} connection failed`);
			}
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const handleToggle = async (checked: boolean) => {
		setEnabled(checked);
		if (!data) return;
		try {
			const result = await updateConnector({ name, enabled: checked, config }).unwrap();
			const connection = (result as { connection?: { ok?: boolean; error?: string } }).connection;
			if (connection) setLastProbe(connection);
			if (!checked) {
				toast.success(`${title} disabled and saved`);
				return;
			}
			if (connection && connection.ok === false) {
				toast.error(connection.error || `${title} enabled but connection failed`);
				return;
			}
			toast.success(connection?.ok ? `${title} enabled and connected` : `${title} enabled and saved`);
		} catch (err) {
			setEnabled(!checked);
			toast.error(getErrorMessage(err));
		}
	};

	return (
		<div className="flex w-full flex-col gap-4">
			<div>
				<h2 className="text-lg font-semibold">{title}</h2>
				<p className="text-muted-foreground text-sm">{description}</p>
				<p className="text-muted-foreground mt-2 text-xs">
					Credentials are stored in the workspace DB. When enabled, inference traces are exported live on each request.
				</p>
			</div>
			<div className="flex items-center justify-between rounded-lg border p-3">
				<Label>Enable connector</Label>
				<Switch checked={enabled} disabled={saving} onCheckedChange={(checked) => void handleToggle(checked)} />
			</div>
			{lastProbe && (
				<div
					className={`rounded-lg border px-3 py-2 text-xs ${
						lastProbe.ok
							? "border-emerald-500/30 bg-emerald-500/10 text-emerald-400"
							: "border-red-500/30 bg-red-500/10 text-red-400"
					}`}
					data-testid={`${name}-connection-status`}
				>
					{lastProbe.ok ? "Connected — last probe succeeded" : `Disconnected — ${lastProbe.error || "probe failed"}`}
				</div>
			)}
			{fields.map((field) => (
				<div key={field.key} className="space-y-1">
					<Label htmlFor={`${name}-${field.key}`}>{field.label}</Label>
					<Input
						id={`${name}-${field.key}`}
						name={`${name}-${field.key}`}
						type={field.type || "text"}
						placeholder={field.placeholder}
						value={config[field.key] || ""}
						autoComplete={field.type === "password" ? "new-password" : "off"}
						data-1p-ignore="true"
						data-lpignore="true"
						onChange={(e) => setConfig((current) => ({ ...current, [field.key]: e.target.value }))}
					/>
				</div>
			))}
			<div className="flex justify-end gap-2">
				{onDelete && (
					<Button variant="outline" onClick={onDelete} disabled={isDeleting || saving || testing}>
						Remove
					</Button>
				)}
				<Button
					variant="outline"
					onClick={() => void runTest()}
					disabled={testing || saving || !data}
					data-testid={`${name}-test-connection`}
				>
					<Cable className="h-4 w-4" />
					{testing ? "Testing…" : "Test connection"}
				</Button>
				<Button onClick={() => void save()} disabled={saving || testing}>
					<Save className="h-4 w-4" />
					{saving ? "Saving…" : "Save connector"}
				</Button>
			</div>
		</div>
	);
}
