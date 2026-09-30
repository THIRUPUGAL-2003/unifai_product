import { ConnectorForm } from "../connectorForm";

interface BigQueryConnectorViewProps {
	onDelete?: () => void;
	isDeleting?: boolean;
}

export default function BigQueryConnectorView({ onDelete, isDeleting }: BigQueryConnectorViewProps) {
	return (
		<ConnectorForm
			name="bigquery"
			title="BigQuery"
			description="Stream inference logs into a BigQuery dataset."
			fields={[
				{ key: "project_id", label: "Project ID" },
				{ key: "dataset", label: "Dataset", placeholder: "unifai" },
				{ key: "table", label: "Table", placeholder: "llm_logs" },
				{ key: "credentials_json", label: "Service account JSON", type: "password" },
			]}
			onDelete={onDelete}
			isDeleting={isDeleting}
		/>
	);
}
