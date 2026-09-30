import { ConnectorForm } from "../connectorForm";

interface KafkaConnectorViewProps {
	onDelete?: () => void;
	isDeleting?: boolean;
}

export default function KafkaConnectorView({ onDelete, isDeleting }: KafkaConnectorViewProps) {
	return (
		<ConnectorForm
			name="kafka"
			title="Kafka"
			description="Publish request and response events to a Kafka topic."
			fields={[
				{ key: "brokers", label: "Brokers", placeholder: "localhost:9092" },
				{ key: "topic", label: "Topic", placeholder: "unifai-logs" },
				{ key: "username", label: "Username" },
				{ key: "password", label: "Password", type: "password" },
			]}
			onDelete={onDelete}
			isDeleting={isDeleting}
		/>
	);
}
