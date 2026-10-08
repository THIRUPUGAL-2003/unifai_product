import { Slider } from "@/components/ui/slider";
import { cn } from "@/lib/utils";
import { useEffect } from "react";
import NumberInput from "../number";
import FieldLabel from "./fieldLabel";
import { Parameter } from "./types";

interface Props {
	field: Parameter;
	config: Record<string, unknown>;
	onChange: (value: any) => void;
	disabled?: boolean;
	onInvalid?: (invalid: boolean, field?: string) => void;
	onClear?: () => void;
	className?: string;
	disabledText?: string;
}

export default function NumberFieldView(props: Props) {
	const { field, config } = props;

	const defaultValue =
		typeof field.default === "number"
			? field.default
			: field.id === "temperature"
				? 0.7
				: field.id === "max_tokens"
					? 4096
					: (field.range?.min ?? 0);

	const rawVal = config[field.id];
	const hasExplicitVal = rawVal !== undefined && rawVal !== null && rawVal !== "";
	const displayVal = hasExplicitVal ? Number(rawVal) : defaultValue;

	const invalid = field.range && hasExplicitVal ? isInvalid(displayVal, field.range) : false;

	useEffect(() => {
		if (!props.onInvalid) return;
		if (invalid) {
			props.onInvalid(true, field.id);
		} else {
			props.onInvalid(false, field.id);
		}
	}, [invalid, field.id, props]);

	const sliderStep =
		field.range?.step ??
		((field.range?.max ?? 1) > 10 ? 1 : 0.01);

	return (
		<div className={cn("flex flex-col gap-3", props.className)}>
			<FieldLabel
				label={field.label}
				helpText={field.helpText}
				onClear={hasExplicitVal && !props.disabled ? props.onClear : undefined}
			>
				{field.range && (
					<NumberInput
						className={cn(
							"ml-auto h-[24px] w-[80px] text-center shrink-0",
							invalid ? "border-border-error focus-visible:ring-border-error" : "",
						)}
						value={displayVal}
						disabled={props.disabled && props.disabled === true}
						onChange={(value) => {
							if (value === undefined) {
								props.onChange(undefined);
							} else {
								props.onChange(Number(value));
							}
						}}
						preventOnBlurFallback
						min={field.range?.min}
						max={field.range?.max}
						step={sliderStep}
					/>
				)}
			</FieldLabel>
			{field.range ? (
				<Slider
					min={field.range?.min ?? 0}
					max={field.range?.max ?? 1}
					step={sliderStep}
					disabled={props.disabled && props.disabled === true}
					value={[displayVal]}
					onValueChange={(value) => {
						props.onChange(Number(value[0]));
					}}
					thumbTooltipText={(props.disabled && props.disabledText) || undefined}
				/>
			) : (
				<NumberInput
					className="w-full"
					value={displayVal}
					disabled={props.disabled && props.disabled === true}
					onChange={(value) => {
						if (value === undefined) {
							props.onChange(undefined);
						} else {
							props.onChange(Number(value));
						}
					}}
					preventOnBlurFallback
				/>
			)}
			{invalid && (
				<div className="text-content-error -mt-2">
					Please keep {field.label} between {field.range?.min} to {field.range?.max}.
				</div>
			)}
		</div>
	);
}

const isInvalid = (value: number, range: { min: number; max: number }): boolean => {
	if (value === undefined || value === null || range?.min === undefined) return false;
	return isNaN(value) || value < range.min || value > range.max;
};