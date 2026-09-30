import { ConnectorForm } from "../connectorForm";

interface NewRelicConnectorViewProps {
	onDelete?: () => void;
	isDeleting?: boolean;
}

export default function NewRelicConnectorView({ onDelete, isDeleting }: NewRelicConnectorViewProps) {
	return (
		<ConnectorForm
			name="newrelic"
			title="New Relic"
			description="Export inference traces to New Relic Logs using your license key and account ID."
			fields={[
				{ key: "api_key", label: "License / API key", type: "password", placeholder: "env.NEW_RELIC_LICENSE_KEY" },
				{ key: "account_id", label: "Account ID", placeholder: "1234567" },
				{ key: "region", label: "Region", placeholder: "US or EU" },
				{ key: "service", label: "Service name", placeholder: "unifai" },
			]}
			onDelete={onDelete}
			isDeleting={isDeleting}
		/>
	);
}
