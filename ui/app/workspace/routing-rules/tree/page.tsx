/**
 * Routing Tree Page
 * Full-canvas read-only routing rules decision tree visualizer.
 */

import { PRODUCT_NAME } from "@/lib/constants/config";
import { RoutingTreeView } from "./views/routingTreeView";

export const metadata = {
	title: `Routing Tree | ${PRODUCT_NAME}`,
	description: "Read-only decision tree visualization of routing rules",
};

export default function RoutingTreePage() {
	return (
		<div className="no-padding-parent no-border-parent h-[calc(100dvh_)] w-full">
			<RoutingTreeView />
		</div>
	);
}