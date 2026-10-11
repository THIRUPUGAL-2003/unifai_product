import { useMemo } from "react";
import { useGetCoreConfigQuery, useUpdateClientMetadataMutation } from "@/lib/store/apis/configApi";
import { DefaultLargePayloadConfig, LargePayloadConfig } from "@enterprise/lib/types/largePayload";

function parseLargePayload(raw: unknown): LargePayloadConfig | undefined {
	if (!raw || typeof raw !== "object") return undefined;
	const obj = raw as Partial<LargePayloadConfig>;
	return {
		enabled: Boolean(obj.enabled),
		request_threshold_bytes: Number(obj.request_threshold_bytes) || DefaultLargePayloadConfig.request_threshold_bytes,
		response_threshold_bytes: Number(obj.response_threshold_bytes) || DefaultLargePayloadConfig.response_threshold_bytes,
		prefetch_size_bytes: Number(obj.prefetch_size_bytes) || DefaultLargePayloadConfig.prefetch_size_bytes,
		max_payload_bytes: Number(obj.max_payload_bytes) || DefaultLargePayloadConfig.max_payload_bytes,
		truncated_log_bytes: Number(obj.truncated_log_bytes) || DefaultLargePayloadConfig.truncated_log_bytes,
	};
}

export const useGetLargePayloadConfigQuery = () => {
	const result = useGetCoreConfigQuery({ fromDB: true });
	const data = useMemo(() => parseLargePayload(result.data?.metadata?.large_payload), [result.data?.metadata]);
	return {
		data,
		isLoading: result.isLoading,
		isError: result.isError,
		error: result.error ?? null,
	};
};

export const useUpdateLargePayloadConfigMutation = (): [
	(config: LargePayloadConfig) => { unwrap: () => Promise<void> },
	{ isLoading: boolean },
] => {
	const [update, state] = useUpdateClientMetadataMutation();
	return [
		(config) => ({
			unwrap: async () => {
				await update({ large_payload: config }).unwrap();
			},
		}),
		{ isLoading: state.isLoading },
	];
};
