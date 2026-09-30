import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { getErrorMessage } from "@/lib/store";
import { useGetConnectorQuery, useUpdateConnectorMutation } from "@enterprise/lib/store/apis/connectorsApi";
import { Save } from "lucide-react";
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
	const [enabled, setEnabled] = useState(false);
	const [config, setConfig] = useState<Record<string, string>>({});

	useEffect(() => {
		if (!data) return;
		setEnabled(!!data.enabled);
		setConfig(data.config || {});
	}, [data]);

	const save = async () => {
		try {
			const result = await updateConnector({ name, enabled, config }).unwrap();
			const connection = (result as { connection?: { ok?: boolean; error?: string } }).connection;
			if (connection && enabled && connection.ok === false) {
				toast.error(connection.error || `${title} saved but connection failed`);
				return;
			}
			if (connection?.ok) {
				toast.success(`${title} connected and saved`);
			} else {
				toast.success(`${title} connector saved`);
			}
		} catch (err) {
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
				<Switch checked={enabled} onCheckedChange={setEnabled} />
			</div>
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
					<Button variant="outline" onClick={onDelete} disabled={isDeleting}>
						Remove
					</Button>
				)}
				<Button onClick={() => void save()} disabled={saving}>
					<Save className="h-4 w-4" />
					{saving ? "Saving…" : "Save connector"}
				</Button>
			</div>
		</div>
	);
}
