import { Button } from "@/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "@/components/ui/dropdownMenu";
import { PlusIcon } from "lucide-react";

export type ConnectorOption = {
	id: string;
	name: string;
	icon: React.ReactNode;
};

interface AddConnectorDropdownProps {
	existingInSidebar: Set<string>;
	knownConnectors: ConnectorOption[];
	onSelectConnector: (id: string) => void;
	disabled?: boolean;
	variant?: "default" | "empty";
}

export function AddConnectorDropdown({
	existingInSidebar,
	knownConnectors,
	onSelectConnector,
	disabled = false,
	variant = "default",
}: AddConnectorDropdownProps) {
	const available = knownConnectors.filter((connector) => !existingInSidebar.has(connector.id));

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="outline"
					size={variant === "empty" ? "default" : "sm"}
					data-testid="add-connector-btn"
					className={variant === "empty" ? "" : "w-full justify-start"}
					aria-label="Add new connector"
					disabled={disabled || available.length === 0}
				>
					<PlusIcon className="h-4 w-4" />
					{variant === "empty" ? <span>Add connector</span> : <div className="text-xs">Add New Connector</div>}
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
				className="custom-scrollbar max-h-[min(70vh,24rem)] min-w-[var(--radix-dropdown-menu-trigger-width)] overflow-y-auto"
				data-testid="add-connector-dropdown"
			>
				{available.map((connector) => (
					<DropdownMenuItem
						key={connector.id}
						data-testid={`add-connector-option-${connector.id}`}
						onSelect={() => onSelectConnector(connector.id)}
					>
						<span className="flex h-4 w-4 items-center justify-center [&_img]:h-4 [&_img]:w-4 [&_svg]:h-4 [&_svg]:w-4">
							{connector.icon}
						</span>
						<span>{connector.name}</span>
					</DropdownMenuItem>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	);
}
