import { ConnectorForm } from "../connectorForm";

interface PubSubConnectorViewProps {
	onDelete?: () => void;
	isDeleting?: boolean;
}

export default function PubSubConnectorView({ onDelete, isDeleting }: PubSubConnectorViewProps) {
	return (
		<ConnectorForm
			name="pubsub"
			title="Pub/Sub"
			description="Publish events to a Google Cloud Pub/Sub topic."
			fields={[
				{ key: "project_id", label: "Project ID" },
				{ key: "topic", label: "Topic", placeholder: "unifai-events" },
				{ key: "credentials_json", label: "Service account JSON", type: "password" },
			]}
			onDelete={onDelete}
			isDeleting={isDeleting}
		/>
	);
}
