import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { usePromptContext } from "@/components/prompts/context";
import { getErrorMessage } from "@/lib/store";
import {
	useCreatePromptDeploymentMutation,
	useDeletePromptDeploymentMutation,
	useGetPromptDeploymentsQuery,
	useUpdatePromptDeploymentMutation,
} from "@enterprise/lib/store/apis/promptDeploymentsApi";
import { PromptDeployment } from "@enterprise/lib/types/workspace";
import { getApiBaseUrl } from "@/lib/utils/port";
import { Rocket, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

interface PromptVersion {
	version_number: number;
}

export default function PromptDeploymentView({ omitTitle }: { omitTitle?: boolean } = {}) {
	const { selectedPromptId, selectedPrompt } = usePromptContext();
	const { data } = useGetPromptDeploymentsQuery(selectedPromptId ? { prompt_id: selectedPromptId } : undefined, {
		skip: !selectedPromptId,
	});
	const [createDeployment] = useCreatePromptDeploymentMutation();
	const [updateDeployment] = useUpdatePromptDeploymentMutation();
	const [deleteDeployment] = useDeletePromptDeploymentMutation();
	const deployments = data?.deployments || [];
	const [versions, setVersions] = useState<PromptVersion[]>([]);
	const [environment, setEnvironment] = useState("production");
	const [versionNumber, setVersionNumber] = useState(1);

	useEffect(() => {
		if (!selectedPromptId) return;
		// Versions are not embedded on GET /prompts/{id}; use the versions endpoint.
		fetch(`${getApiBaseUrl()}/prompt-repo/prompts/${selectedPromptId}/versions`, { credentials: "include" })
			.then((res) => (res.ok ? res.json() : null))
			.then((data) => {
				const list = (data?.versions || []) as PromptVersion[];
				setVersions(list);
				if (list[0]?.version_number) setVersionNumber(list[0].version_number);
			})
			.catch(() => undefined);
	}, [selectedPromptId]);

	if (!selectedPromptId) {
		return <p className="text-muted-foreground text-sm">Select a prompt to manage deployments.</p>;
	}

	const create = async () => {
		if (!selectedPromptId) return;
		if (versions.length === 0 && !versionNumber) {
			toast.error("Create a prompt version before deploying");
			return;
		}
		try {
			await createDeployment({
				prompt_id: selectedPromptId,
				prompt_name: selectedPrompt?.name || "",
				version_number: versionNumber,
				environment,
				enabled: true,
			}).unwrap();
			toast.success(`Deployed to ${environment}`);
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const toggle = async (deployment: PromptDeployment) => {
		try {
			await updateDeployment({ ...deployment, enabled: !deployment.enabled }).unwrap();
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const remove = async (id: number) => {
		try {
			await deleteDeployment(id).unwrap();
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	return (
		<div className="space-y-3">
			{!omitTitle && (
				<h2 className="flex items-center gap-2 text-sm font-semibold">
					<Rocket className="h-4 w-4" />
					Deployments
				</h2>
			)}
			<p className="text-muted-foreground text-xs">
				API calls can load this version with header <code className="text-[10px]">x-uf-prompt-environment</code> (or{" "}
				<code className="text-[10px]">x-uf-dim-environment</code>) matching the environment name below.
			</p>
			<div className="space-y-2">
				<Label className="text-xs">Environment</Label>
				<select value={environment} onChange={(e) => setEnvironment(e.target.value)} className="border-input bg-background h-8 w-full rounded-md border px-2 text-xs">
					<option value="production">production</option>
					<option value="staging">staging</option>
					<option value="preview">preview</option>
				</select>
				<Label className="text-xs">Version</Label>
				{versions.length > 0 ? (
					<select
						value={versionNumber}
						onChange={(e) => setVersionNumber(Number(e.target.value))}
						className="border-input bg-background h-8 w-full rounded-md border px-2 text-xs"
					>
						{versions.map((version) => (
							<option key={version.version_number} value={version.version_number}>
								v{version.version_number}
							</option>
						))}
					</select>
				) : (
					<Input type="number" value={versionNumber} onChange={(e) => setVersionNumber(Number(e.target.value))} />
				)}
				<Button size="sm" className="w-full" onClick={() => void create()}>
					Deploy version
				</Button>
			</div>
			<div className="space-y-2">
				{deployments.map((deployment) => (
					<div key={deployment.id} className="rounded-lg border p-2 text-xs">
						<div className="flex items-center justify-between">
							<div>
								<div className="font-medium">{deployment.environment}</div>
								<Badge variant="secondary">v{deployment.version_number}</Badge>
							</div>
							<div className="flex items-center gap-1">
								<Switch checked={deployment.enabled} onCheckedChange={() => void toggle(deployment)} />
								<Button size="icon" variant="ghost" onClick={() => void remove(deployment.id)}>
									<Trash2 className="h-3.5 w-3.5" />
								</Button>
							</div>
						</div>
					</div>
				))}
			</div>
		</div>
	);
}
