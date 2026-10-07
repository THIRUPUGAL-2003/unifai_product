import { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertCircle } from "lucide-react";
import { Button } from "@/components/ui/button";

interface Props {
	children: ReactNode;
	/** Shown above the retry action */
	title?: string;
}

interface State {
	hasError: boolean;
	message: string;
}

/**
 * Catches render crashes in workspace routes/sheets so one bad view
 * does not blank the entire ClientLayout outlet.
 */
export class WorkspaceErrorBoundary extends Component<Props, State> {
	constructor(props: Props) {
		super(props);
		this.state = { hasError: false, message: "" };
	}

	static getDerivedStateFromError(error: Error): State {
		return { hasError: true, message: error?.message || "Unexpected error" };
	}

	componentDidCatch(error: Error, _info: ErrorInfo) {
		console.error("[WorkspaceErrorBoundary]", error);
	}

	private reset = () => {
		this.setState({ hasError: false, message: "" });
	};

	render() {
		if (this.state.hasError) {
			return (
				<div
					className="flex h-full min-h-[40vh] flex-col items-center justify-center gap-3 p-6 text-center"
					data-testid="workspace-error-boundary"
				>
					<AlertCircle className="text-destructive h-8 w-8" />
					<p className="text-foreground text-sm font-semibold">{this.props.title || "This page crashed"}</p>
					<p className="text-muted-foreground max-w-md text-xs break-words">{this.state.message}</p>
					<div className="flex items-center gap-2">
						<Button type="button" variant="outline" size="sm" onClick={this.reset}>
							Try again
						</Button>
						<Button type="button" size="sm" onClick={() => window.location.reload()}>
							Reload
						</Button>
					</div>
				</div>
			);
		}
		return this.props.children;
	}
}
