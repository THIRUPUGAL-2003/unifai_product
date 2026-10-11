import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { LargePayloadConfig } from "@enterprise/lib/types/largePayload";

export interface LargePayloadSettingsFragmentProps {
	config: LargePayloadConfig;
	onConfigChange: (config: LargePayloadConfig) => void;
	controlsDisabled: boolean;
}

const fields: { key: keyof LargePayloadConfig; label: string; hint: string }[] = [
	{ key: "request_threshold_bytes", label: "Request threshold (bytes)", hint: "Bodies larger than this stream instead of being buffered." },
	{ key: "response_threshold_bytes", label: "Response threshold (bytes)", hint: "Stored with the policy for response streaming." },
	{ key: "prefetch_size_bytes", label: "Prefetch size (bytes)", hint: "Bytes read first so model metadata can be parsed." },
	{ key: "max_payload_bytes", label: "Max payload (bytes)", hint: "Larger requests are rejected on the next call." },
	{ key: "truncated_log_bytes", label: "Truncated log (bytes)", hint: "How much of a large body is kept in logs." },
];

export default function LargePayloadSettingsFragment({ config, onConfigChange, controlsDisabled }: LargePayloadSettingsFragmentProps) {
	return (
		<div className="space-y-4">
			<div className="flex items-center justify-between space-x-2">
				<div className="space-y-0.5">
					<label htmlFor="large-payload-enabled" className="text-sm font-medium">
						Large payload handling
					</label>
					<p className="text-muted-foreground text-sm">
						Applies on the next request. Oversized bodies stream, and anything above the max payload is rejected.
					</p>
				</div>
				<Switch
					id="large-payload-enabled"
					size="md"
					checked={config.enabled}
					disabled={controlsDisabled}
					onCheckedChange={(checked) => onConfigChange({ ...config, enabled: checked })}
				/>
			</div>
			<div className="grid gap-3 sm:grid-cols-2">
				{fields.map((field) => (
					<div key={field.key} className="space-y-1">
						<label htmlFor={`large-payload-${field.key}`} className="text-sm font-medium">
							{field.label}
						</label>
						<Input
							id={`large-payload-${field.key}`}
							type="number"
							min={1024}
							disabled={controlsDisabled || !config.enabled}
							value={config[field.key] as number}
							onChange={(e) => onConfigChange({ ...config, [field.key]: Number.parseInt(e.target.value, 10) || 0 })}
						/>
						<p className="text-muted-foreground text-xs">{field.hint}</p>
					</div>
				))}
			</div>
		</div>
	);
}
