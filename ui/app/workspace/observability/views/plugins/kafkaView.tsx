import KafkaConnectorView from "@enterprise/components/data-connectors/kafka/kafkaConnectorView";

interface KafkaViewProps {
	onDelete?: () => void;
	isDeleting?: boolean;
}

export default function KafkaView({ onDelete, isDeleting }: KafkaViewProps) {
	return (
		<div className="flex w-full flex-col gap-4">
			<div className="flex w-full flex-col gap-3">
				<KafkaConnectorView onDelete={onDelete} isDeleting={isDeleting} />
			</div>
		</div>
	);
}
