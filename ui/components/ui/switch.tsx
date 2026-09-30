import * as SwitchPrimitives from "@radix-ui/react-switch";
import { Loader2 } from "lucide-react";
import * as React from "react";

import { cn } from "@/lib/utils";

interface SwitchProps extends React.ComponentPropsWithoutRef<typeof SwitchPrimitives.Root> {
	size?: "default" | "md";
	onAsyncCheckedChange?: (checked: boolean) => Promise<void>;
}

const Switch = React.forwardRef<React.ElementRef<typeof SwitchPrimitives.Root>, SwitchProps>(
	({ className, size = "md", onAsyncCheckedChange, onCheckedChange, disabled, ...props }, ref) => {
		const [loading, setLoading] = React.useState(false);

		const handleCheckedChange = React.useCallback(
			(checked: boolean) => {
				if (onAsyncCheckedChange) {
					setLoading(true);
					Promise.resolve()
						.then(() => onAsyncCheckedChange(checked))
						.catch(() => {})
						.finally(() => {
							setLoading(false);
						});
				} else {
					onCheckedChange?.(checked);
				}
			},
			[onAsyncCheckedChange, onCheckedChange],
		);

		return (
			<SwitchPrimitives.Root
				className={cn(
					"peer focus-visible:ring-ring focus-visible:ring-offset-background inline-flex shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent p-0.5 transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50",
					"data-[state=checked]:bg-emerald-600 data-[state=unchecked]:bg-zinc-600 dark:data-[state=unchecked]:bg-zinc-700",
					size === "default" && "h-6 w-11",
					size === "md" && "h-5 w-9",
					className,
				)}
				disabled={disabled || loading}
				onCheckedChange={handleCheckedChange}
				{...props}
				ref={ref}
			>
				<SwitchPrimitives.Thumb
					className={cn(
						"pointer-events-none relative block rounded-full bg-white shadow-md ring-0 transition-transform",
						size === "default" && "h-4 w-4 data-[state=checked]:translate-x-5 data-[state=unchecked]:translate-x-0",
						size === "md" && "h-3.5 w-3.5 data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0",
					)}
				>
					{loading && (
						<Loader2
							className={cn(
								"absolute inset-0 m-auto animate-spin text-zinc-500",
								size === "default" && "h-3 w-3",
								size === "md" && "h-2.5 w-2.5",
							)}
						/>
					)}
				</SwitchPrimitives.Thumb>
			</SwitchPrimitives.Root>
		);
	},
);
Switch.displayName = SwitchPrimitives.Root.displayName;

export { Switch };
