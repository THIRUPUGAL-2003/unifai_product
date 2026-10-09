import React from "react";
import { Button } from "@/components/ui/button";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from "lucide-react";

export interface DataTablePaginationProps {
	/** Current 0-based offset of items */
	offset: number;
	/** Number of items per page (limit) */
	limit: number;
	/** Total count of items matching the query/filter */
	totalCount: number;
	/** Callback when offset changes (page navigation) */
	onOffsetChange: (newOffset: number) => void;
	/** Callback when page size changes (optional) */
	onLimitChange?: (newLimit: number) => void;
	/** Available page size options (default: [10, 25, 50, 100]) */
	pageSizeOptions?: number[];
	/** Custom label for items, e.g. "entries", "parent domains", "logs", "keys" */
	itemLabel?: string;
	/** Custom prefix for the page size selector, e.g. "Rows per page" or "Parent domains per page" */
	perPageLabel?: string;
	/** Show first/last page jump buttons (default: false) */
	showFirstLast?: boolean;
	/** Optional additional CSS classes */
	className?: string;
	/** Custom data-testid */
	dataTestId?: string;
}

export function DataTablePagination({
	offset,
	limit,
	totalCount,
	onOffsetChange,
	onLimitChange,
	pageSizeOptions = [10, 25, 50, 100],
	itemLabel = "entries",
	perPageLabel = "Rows per page",
	showFirstLast = false,
	className = "",
	dataTestId = "pagination",
}: DataTablePaginationProps) {
	if (totalCount <= 0) {
		return null;
	}

	const safeLimit = Math.max(1, limit);
	const totalPages = Math.max(1, Math.ceil(totalCount / safeLimit));
	const currentPage = Math.min(totalPages, Math.floor(offset / safeLimit) + 1);

	const startItem = totalCount > 0 ? offset + 1 : 0;
	const endItem = Math.min(offset + safeLimit, totalCount);

	const handlePageChange = (newPage: number) => {
		const targetPage = Math.max(1, Math.min(totalPages, newPage));
		const newOffset = (targetPage - 1) * safeLimit;
		onOffsetChange(newOffset);
	};

	const handleLimitChange = (valStr: string) => {
		const newLimit = Number(valStr);
		if (onLimitChange && !isNaN(newLimit) && newLimit > 0) {
			onLimitChange(newLimit);
			// Reset offset to 0 when page size changes
			onOffsetChange(0);
		}
	};

	return (
		<div
			className={`flex flex-col sm:flex-row items-center justify-between gap-4 pt-3.5 pb-1 border-t border-border/50 mt-3 text-xs text-muted-foreground select-none ${className}`}
			data-testid={dataTestId}
		>
			{/* Left section: Page size selector and item range counter */}
			<div className="flex flex-wrap items-center gap-3">
				{onLimitChange && (
					<div className="inline-flex items-center gap-2">
						<span className="whitespace-nowrap text-xs font-medium text-muted-foreground leading-none">
							{perPageLabel}
						</span>
						<Select value={String(safeLimit)} onValueChange={handleLimitChange}>
							<SelectTrigger
								size="sm"
								className="h-7 min-h-7 w-[64px] rounded-md border-border/70 bg-background/60 hover:bg-background text-xs font-semibold text-foreground px-2 py-0 focus:ring-1 focus:ring-teal-500/30 transition-all cursor-pointer shadow-2xs"
								aria-label={perPageLabel}
								data-testid={`${dataTestId}-limit-select`}
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent className="min-w-[64px]">
								{pageSizeOptions.map((sz) => (
									<SelectItem key={sz} value={String(sz)} className="text-xs">
										{sz}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
				)}
				{onLimitChange && (
					<span className="text-muted-foreground/30 font-bold select-none">•</span>
				)}
				<span className="whitespace-nowrap text-xs text-muted-foreground leading-none">
					Showing <span className="font-semibold text-foreground">{startItem.toLocaleString()}</span> to{" "}
					<span className="font-semibold text-foreground">{endItem.toLocaleString()}</span> of{" "}
					<span className="font-semibold text-foreground">{totalCount.toLocaleString()}</span> {itemLabel}
				</span>
			</div>

			{/* Right section: Page status and navigation buttons */}
			<div className="flex items-center gap-3">
				<span className="whitespace-nowrap text-xs text-muted-foreground leading-none">
					Page <span className="font-semibold text-foreground">{currentPage}</span> of{" "}
					<span className="font-semibold text-foreground">{totalPages}</span>
				</span>
				<div className="flex items-center gap-1">
					{showFirstLast && (
						<Button
							variant="outline"
							size="icon"
							disabled={currentPage <= 1}
							onClick={() => handlePageChange(1)}
							className="h-7 w-7 rounded-md border-border/70 hover:bg-muted/60 transition-colors shadow-2xs"
							title="First page"
							aria-label="First page"
						>
							<ChevronsLeft className="h-3.5 w-3.5" />
						</Button>
					)}
					<Button
						variant="outline"
						size="icon"
						disabled={currentPage <= 1}
						onClick={() => handlePageChange(currentPage - 1)}
						className="h-7 w-7 rounded-md border-border/70 hover:bg-muted/60 transition-colors shadow-2xs"
						data-testid={`${dataTestId}-prev-btn`}
						title="Previous page"
						aria-label="Previous page"
					>
						<ChevronLeft className="h-3.5 w-3.5" />
					</Button>

					{/* Numerical Quick Jump buttons when totalPages is moderate */}
					{totalPages <= 7 && totalPages > 1 && (
						<div className="hidden md:flex items-center gap-1 mx-0.5">
							{Array.from({ length: totalPages }, (_, i) => i + 1).map((p) => (
								<Button
									key={p}
									variant={p === currentPage ? "default" : "outline"}
									size="sm"
									onClick={() => handlePageChange(p)}
									className={`h-7 w-7 p-0 rounded-md text-xs transition-colors shadow-2xs ${
										p === currentPage ? "bg-teal-500 hover:bg-teal-600 text-white font-bold" : "border-border/70 hover:bg-muted/60"
									}`}
								>
									{p}
								</Button>
							))}
						</div>
					)}

					<Button
						variant="outline"
						size="icon"
						disabled={currentPage >= totalPages}
						onClick={() => handlePageChange(currentPage + 1)}
						className="h-7 w-7 rounded-md border-border/70 hover:bg-muted/60 transition-colors shadow-2xs"
						data-testid={`${dataTestId}-next-btn`}
						title="Next page"
						aria-label="Next page"
					>
						<ChevronRight className="h-3.5 w-3.5" />
					</Button>
					{showFirstLast && (
						<Button
							variant="outline"
							size="icon"
							disabled={currentPage >= totalPages}
							onClick={() => handlePageChange(totalPages)}
							className="h-7 w-7 rounded-md border-border/70 hover:bg-muted/60 transition-colors shadow-2xs"
							title="Last page"
							aria-label="Last page"
						>
							<ChevronsRight className="h-3.5 w-3.5" />
						</Button>
					)}
				</div>
			</div>
		</div>
	);
}

export default DataTablePagination;
