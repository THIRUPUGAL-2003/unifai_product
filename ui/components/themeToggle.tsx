import { Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { useEffect } from "react";

import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdownMenu";

export function ThemeToggle() {
	const { theme, setTheme } = useTheme();

	// Only Light / Dark — migrate any leftover "system" preference.
	useEffect(() => {
		if (theme === "system") {
			setTheme("light");
		}
	}, [theme, setTheme]);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="ghost"
					size="icon"
					className="hover:text-primary text-muted-foreground h-5 w-5 border-0 ring-offset-0 outline-none select-none focus-visible:ring-0"
				>
					<Sun className="h-5.5 w-5.5 scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90" strokeWidth={2} />
					<Moon className="absolute h-5.5 w-5.5 scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0" strokeWidth={2} />
					<span className="sr-only">Toggle theme</span>
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end">
				<DropdownMenuItem onClick={() => setTheme("light")}>Light</DropdownMenuItem>
				<DropdownMenuItem onClick={() => setTheme("dark")}>Dark</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
}
