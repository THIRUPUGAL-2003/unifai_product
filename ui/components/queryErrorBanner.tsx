import { cn } from "@/lib/utils";

/** Inline banner when RTK queries fail (avoids looking like empty data). */
export function QueryErrorBanner({
	message = "Some data failed to load. Try refreshing or adjusting filters.",
	className,
	testId = "query-error-banner",
}: {
	message?: string;
	className?: string;
	testId?: string;
}) {
	return (
		<div
			className={cn("border-destructive/30 bg-destructive/5 text-destructive rounded-md border px-3 py-2 text-xs", className)}
			data-testid={testId}
			role="alert"
		>
			{message}
		</div>
	);
}
