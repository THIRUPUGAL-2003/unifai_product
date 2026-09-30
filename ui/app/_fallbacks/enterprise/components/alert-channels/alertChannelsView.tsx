import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { getErrorMessage } from "@/lib/store";
import {
	useCreateAlertChannelMutation,
	useDeleteAlertChannelMutation,
	useGetAlertChannelsQuery,
	useTestAlertChannelMutation,
	useUpdateAlertChannelMutation,
} from "@enterprise/lib/store/apis/alertChannelsApi";
import { AlertChannel } from "@enterprise/lib/types/workspace";
import { Bell, Plus, Send, Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

const emptyChannel = (): Omit<AlertChannel, "id"> => ({
	name: "",
	type: "webhook",
	enabled: true,
	config: { url: "" },
});

export default function AlertChannelsView() {
	const [open, setOpen] = useState(false);
	const [form, setForm] = useState(emptyChannel());
	const { data, isLoading: loading } = useGetAlertChannelsQuery();
	const [createChannel] = useCreateAlertChannelMutation();
	const [updateChannel] = useUpdateAlertChannelMutation();
	const [testChannel] = useTestAlertChannelMutation();
	const [deleteChannel] = useDeleteAlertChannelMutation();
	const channels = data?.channels || [];

	const save = async () => {
		try {
			await createChannel(form).unwrap();
			toast.success("Alert channel created");
			setOpen(false);
			setForm(emptyChannel());
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const toggle = async (channel: AlertChannel) => {
		try {
			await updateChannel({ ...channel, enabled: !channel.enabled }).unwrap();
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const test = async (id: number) => {
		try {
			const data = await testChannel(id).unwrap();
			toast.success(data.message);
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	const remove = async (id: number) => {
		try {
			await deleteChannel(id).unwrap();
			toast.success("Channel deleted");
		} catch (err) {
			toast.error(getErrorMessage(err));
		}
	};

	return (
		<div className="flex w-full flex-col gap-6 p-1">
			<div className="flex items-center justify-between">
				<div>
					<h1 className="flex items-center gap-2 text-2xl font-semibold">
						<Bell className="h-6 w-6" />
						Alert Channels
					</h1>
					<p className="text-muted-foreground mt-1 text-sm">Webhook, Slack, email, or PagerDuty destinations for workspace alerts.</p>
				</div>
				<Button onClick={() => setOpen(true)}>
					<Plus className="h-4 w-4" />
					Add channel
				</Button>
			</div>

			{loading ? (
				<p className="text-muted-foreground text-sm">Loading channels…</p>
			) : channels.length === 0 ? (
				<div className="rounded-xl border border-dashed p-10 text-center">
					<p className="font-medium">No alert channels</p>
					<p className="text-muted-foreground mt-1 text-sm">Add a destination to start receiving operational alerts.</p>
				</div>
			) : (
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Name</TableHead>
							<TableHead>Type</TableHead>
							<TableHead>Enabled</TableHead>
							<TableHead className="text-right">Actions</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{channels.map((channel) => (
							<TableRow key={channel.id}>
								<TableCell className="font-medium">{channel.name}</TableCell>
								<TableCell>
									<Badge variant="secondary">{channel.type}</Badge>
								</TableCell>
								<TableCell>
									<Switch checked={channel.enabled} onCheckedChange={() => void toggle(channel)} />
								</TableCell>
								<TableCell className="text-right">
									<Button size="icon" variant="ghost" onClick={() => void test(channel.id)}>
										<Send className="h-4 w-4" />
									</Button>
									<Button size="icon" variant="ghost" onClick={() => void remove(channel.id)}>
										<Trash2 className="h-4 w-4" />
									</Button>
								</TableCell>
							</TableRow>
						))}
					</TableBody>
				</Table>
			)}

			<Dialog open={open} onOpenChange={setOpen}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>New alert channel</DialogTitle>
					</DialogHeader>
					<div className="space-y-3 py-2">
						<div className="space-y-1">
							<Label>Name</Label>
							<Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
						</div>
						<div className="space-y-1">
							<Label>Type</Label>
							<select
								value={form.type}
								onChange={(e) => setForm({ ...form, type: e.target.value })}
								className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
							>
								<option value="webhook">Webhook</option>
								<option value="slack">Slack</option>
								<option value="email">Email</option>
								<option value="pagerduty">PagerDuty</option>
							</select>
						</div>
						<div className="space-y-1">
							<Label>{form.type === "email" ? "Email address" : form.type === "pagerduty" ? "Routing key" : "URL"}</Label>
							<Input
								value={form.config.url || form.config.address || form.config.routing_key || ""}
								onChange={(e) => setForm({ ...form, config: { url: e.target.value, address: e.target.value, routing_key: e.target.value } })}
							/>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setOpen(false)}>
							Cancel
						</Button>
						<Button onClick={() => void save()}>Create</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
