import { Button } from "@/components/ui/button";
import { Link } from "@tanstack/react-router";
import { SlidersHorizontal } from "lucide-react";

interface PricingOverridesEmptyStateProps {
	onCreateClick: () => void;
	canCreate?: boolean;
}

export function PricingOverridesEmptyState({ onCreateClick, canCreate = true }: PricingOverridesEmptyStateProps) {
	return (
		<div
			className="flex min-h-[80vh] w-full flex-col items-center justify-center gap-4 py-16 text-center"
			data-testid="pricing-overrides-empty-state"
		>
			<div className="text-muted-foreground">
				<SlidersHorizontal className="h-[5.5rem] w-[5.5rem]" strokeWidth={1} />
			</div>
			<div className="flex flex-col gap-1">
				<h1 className="text-muted-foreground text-xl font-medium">Pricing overrides customize cost tracking per scope</h1>
				<div className="text-muted-foreground mx-auto mt-2 max-w-[600px] text-sm font-normal">
					Define custom per-token prices for specific providers, keys, or virtual keys to accurately reflect your negotiated rates. Base
					catalog prices still come from{" "}
					<Link to="/workspace/custom-pricing" className="text-primary underline underline-offset-2">
						Model Settings
					</Link>{" "}
					sync.
				</div>
				<div className="mx-auto mt-6 flex flex-row flex-wrap items-center justify-center gap-2">
					<Button
						aria-label="Create your first pricing override"
						data-testid="pricing-override-create-btn"
						onClick={onCreateClick}
						disabled={!canCreate}
					>
						Create Override
					</Button>
				</div>
			</div>
		</div>
	);
}
