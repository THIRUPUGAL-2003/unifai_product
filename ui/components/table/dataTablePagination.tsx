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
			className={`flex flex-col sm:flex-row items-center justify-between gap-4 pt-4 border-t border-border mt-4 text-xs text-muted-foreground select-none ${className}`}
			data-testid={dataTestId}
		>
			{/* Left section: Page size selector and item range counter */}
			<div className="flex flex-wrap items-center gap-2.5">
				{onLimitChange && (
					<div className="flex items-center gap-2">
						<span className="whitespace-nowrap font-medium">{perPageLabel}</span>
						<Select value={String(safeLimit)} onValueChange={handleLimitChange}>
							<SelectTrigger
								className="h-8 w-[72px] bg-background border-border text-xs focus:ring-1"
								aria-label={perPageLabel}
								data-testid={`${dataTestId}-limit-select`}
							>
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{pageSizeOptions.map((sz) => (
									<SelectItem key={sz} value={String(sz)} className="text-xs">
										{sz}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
				)}
				<span className="whitespace-nowrap">
					Showing <span className="font-semibold text-foreground">{startItem.toLocaleString()}</span> to{" "}
					<span className="font-semibold text-foreground">{endItem.toLocaleString()}</span> of{" "}
					<span className="font-semibold text-foreground">{totalCount.toLocaleString()}</span> {itemLabel}
				</span>
			</div>

			{/* Right section: Page status and navigation buttons */}
			<div className="flex items-center gap-2">
				<span className="whitespace-nowrap">
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
							className="h-8 w-8 border-border hover:bg-muted"
							title="First page"
							aria-label="First page"
						>
							<ChevronsLeft className="h-4 w-4" />
						</Button>
					)}
					<Button
						variant="outline"
						size="icon"
						disabled={currentPage <= 1}
						onClick={() => handlePageChange(currentPage - 1)}
						className="h-8 w-8 border-border hover:bg-muted"
						data-testid={`${dataTestId}-prev-btn`}
						title="Previous page"
						aria-label="Previous page"
					>
						<ChevronLeft className="h-4 w-4" />
					</Button>

					{/* Numerical Quick Jump buttons when totalPages is moderate */}
					{totalPages <= 7 && totalPages > 1 && (
						<div className="hidden md:flex items-center gap-1 mx-1">
							{Array.from({ length: totalPages }, (_, i) => i + 1).map((p) => (
								<Button
									key={p}
									variant={p === currentPage ? "default" : "outline"}
									size="sm"
									onClick={() => handlePageChange(p)}
									className={`h-8 w-8 p-0 text-xs ${
										p === currentPage ? "font-bold shadow-sm" : "border-border hover:bg-muted"
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
						className="h-8 w-8 border-border hover:bg-muted"
						data-testid={`${dataTestId}-next-btn`}
						title="Next page"
						aria-label="Next page"
					>
						<ChevronRight className="h-4 w-4" />
					</Button>
					{showFirstLast && (
						<Button
							variant="outline"
							size="icon"
							disabled={currentPage >= totalPages}
							onClick={() => handlePageChange(totalPages)}
							className="h-8 w-8 border-border hover:bg-muted"
							title="Last page"
							aria-label="Last page"
						>
							<ChevronsRight className="h-4 w-4" />
						</Button>
					)}
				</div>
			</div>
		</div>
	);
}

export default DataTablePagination;
