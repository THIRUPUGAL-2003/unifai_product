import NewRelicConnectorView from "@enterprise/components/data-connectors/newrelic/newRelicConnectorView";

interface NewrelicViewProps {
	onDelete?: () => void;
	isDeleting?: boolean;
}

export default function NewrelicView({ onDelete, isDeleting }: NewrelicViewProps) {
	return (
		<div className="flex w-full flex-col gap-4">
			<div className="flex w-full flex-col gap-3">
				<NewRelicConnectorView onDelete={onDelete} isDeleting={isDeleting} />
			</div>
		</div>
	);
}
