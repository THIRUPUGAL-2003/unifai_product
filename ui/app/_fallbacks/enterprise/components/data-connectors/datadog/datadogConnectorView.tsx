import { ConnectorForm } from "../connectorForm";

interface EnableToggleProps {
	enabled: boolean;
	onToggle: () => void;
	disabled?: boolean;
}

interface DatadogConnectorViewProps {
	onDelete?: () => void;
	isDeleting?: boolean;
	enableToggle?: EnableToggleProps;
}

export default function DatadogConnectorView({ onDelete, isDeleting }: DatadogConnectorViewProps) {
	return (
		<ConnectorForm
			name="datadog"
			title="Datadog"
			description="Export traces and metrics to Datadog using an API key and site."
			fields={[
				{ key: "api_key", label: "API key", type: "password", placeholder: "env.DATADOG_API_KEY" },
				{ key: "site", label: "Site", placeholder: "datadoghq.com" },
				{ key: "service", label: "Service name", placeholder: "unifai" },
			]}
			onDelete={onDelete}
			isDeleting={isDeleting}
		/>
	);
}
