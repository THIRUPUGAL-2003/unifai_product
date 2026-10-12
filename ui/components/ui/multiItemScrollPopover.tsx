import React from "react";
import { Badge } from "@/components/ui/badge";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

export interface PopoverItem {
	id?: string | number;
	label: string;
	icon?: React.ReactNode;
}

export interface MultiItemScrollPopoverProps {
	items: Array<PopoverItem | string>;
	pluralLabel?: string;
	emptyText?: string;
	popoverTitle?: string;
	icon?: React.ReactNode;
	className?: string;
	badgeVariant?: "secondary" | "outline" | "default";
}

export function MultiItemScrollPopover({
	items,
	pluralLabel = "Items",
	emptyText = "None",
	popoverTitle,
	icon,
	className,
	badgeVariant = "secondary",
}: MultiItemScrollPopoverProps) {
	const normalizedItems: PopoverItem[] = React.useMemo(() => {
		return (items || [])
			.map((item) => {
				if (typeof item === "string") {
					return { id: item, label: item };
				}
				return item;
			})
			.filter((item) => Boolean(item && item.label));
	}, [items]);

	if (normalizedItems.length === 0) {
		return <span className={cn("text-muted-foreground text-xs italic", className)}>{emptyText}</span>;
	}

	if (normalizedItems.length === 1) {
		const single = normalizedItems[0];
		return (
			<Badge
				variant={badgeVariant}
				className={cn(
					"text-[11px] font-normal py-0 px-1.5 flex items-center gap-1 w-fit max-w-[180px]",
					className,
				)}
				title={single.label}
			>
				{single.icon || icon}
				<span className="truncate">{single.label}</span>
			</Badge>
		);
	}

	return (
		<Popover>
			<PopoverTrigger asChild>
				<button
					type="button"
					className={cn(
						"inline-flex items-center gap-1 rounded-sm border border-border bg-secondary/80 hover:bg-secondary px-2 py-0.5 text-[11px] font-medium text-secondary-foreground transition-colors cursor-pointer shadow-2xs focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring",
						className,
					)}
					title="Click to view all"
				>
					{icon}
					<span>
						{normalizedItems.length} {pluralLabel}
					</span>
					<ChevronDown className="h-3 w-3 text-muted-foreground ml-0.5 opacity-70 shrink-0" />
				</button>
			</PopoverTrigger>
			<PopoverContent className="w-64 p-3 z-50 shadow-md" align="start" sideOffset={6}>
				<div className="flex items-center justify-between pb-2 mb-2 border-b text-xs font-medium text-muted-foreground">
					<span>{popoverTitle || `${pluralLabel} (${normalizedItems.length})`}</span>
					<Badge variant="outline" className="text-[10px] px-1.5 py-0 h-4">
						{normalizedItems.length}
					</Badge>
				</div>
				<div
					className="max-h-48 overflow-y-auto space-y-1 pr-1 overscroll-contain"
					style={{ scrollbarWidth: "thin" }}
				>
					{normalizedItems.map((item, idx) => (
						<div
							key={item.id ?? idx}
							className="flex items-center gap-2 p-1.5 rounded-md hover:bg-muted/60 text-xs transition-colors"
							title={item.label}
						>
							{item.icon || icon}
							<span className="truncate font-medium text-foreground">{item.label}</span>
						</div>
					))}
				</div>
			</PopoverContent>
		</Popover>
	);
}
