import BigQueryConnectorView from "@enterprise/components/data-connectors/bigquery/bigqueryConnectorView";

interface BigQueryViewProps {
	onDelete?: () => void;
	isDeleting?: boolean;
}

export default function BigQueryView({ onDelete, isDeleting }: BigQueryViewProps) {
	return (
		<div className="flex w-full flex-col gap-4">
			<div className="flex w-full flex-col gap-3">
				<BigQueryConnectorView onDelete={onDelete} isDeleting={isDeleting} />
			</div>
		</div>
	);
}
