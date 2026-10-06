import { useState, useEffect } from "react";
import {
	PRODUCT_NAME,
	PRODUCT_FULL_NAME,
	COMPANY_NAME,
	COMPANY_SHORT_NAME,
	COMPANY_LOGO,
	DEFAULT_FOOTER_TEXT,
	FOOTER_SUBTITLE,
} from "@/lib/constants/config";
import { getApiBaseUrl } from "@/lib/utils/port";

export interface BrandingInfo {
	productName: string;
	productFullName: string;
	companyName: string;
	companyShortName: string;
	companyLogo: string;
	footerText: string;
	footerSubtitle: string;
}

export function useBranding(): BrandingInfo {
	const [branding, setBranding] = useState<BrandingInfo>({
		productName: PRODUCT_NAME,
		productFullName: PRODUCT_FULL_NAME,
		companyName: COMPANY_NAME,
		companyShortName: COMPANY_SHORT_NAME,
		companyLogo: COMPANY_LOGO,
		footerText: DEFAULT_FOOTER_TEXT,
		footerSubtitle: FOOTER_SUBTITLE,
	});

	useEffect(() => {
		fetch(`${getApiBaseUrl()}/api/branding`)
			.then((res) => (res.ok ? res.json() : null))
			.then((data) => {
				if (!data) return;
				setBranding({
					productName: data.product_name || PRODUCT_NAME,
					productFullName: data.product_subtitle || PRODUCT_FULL_NAME,
					companyName: data.company_name || COMPANY_NAME,
					companyShortName: data.company_short_name || COMPANY_SHORT_NAME,
					companyLogo: data.company_logo || COMPANY_LOGO,
					footerText: data.footer_copyright || DEFAULT_FOOTER_TEXT,
					footerSubtitle: data.footer_subtitle || FOOTER_SUBTITLE,
				});
			})
			.catch(() => {
				// Keep fallback constants
			});
	}, []);

	return branding;
}
