import PubSubConnectorView from "@enterprise/components/data-connectors/pubsub/pubsubConnectorView";

interface PubSubViewProps {
	onDelete?: () => void;
	isDeleting?: boolean;
}

export default function PubSubView({ onDelete, isDeleting }: PubSubViewProps) {
	return (
		<div className="flex w-full flex-col gap-4">
			<div className="flex w-full flex-col gap-3">
				<PubSubConnectorView onDelete={onDelete} isDeleting={isDeleting} />
			</div>
		</div>
	);
}
