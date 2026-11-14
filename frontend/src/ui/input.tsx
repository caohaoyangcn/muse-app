import * as React from "react";
import { TextInput } from "@astryxdesign/core/TextInput";

/**
 * The one text field in the app, on Astryx.
 *
 * Keeps the native `<input>` prop shape (`value` + `onChange(event)`, `id`,
 * `placeholder`, `type`, `ref`) so the ~15 form blocks that already markup their
 * own `<label htmlFor>` keep working: Astryx forwards `id` to the real input, so
 * the association survives.
 *
 * Two translations happen here:
 *
 * - Astryx reports `onChange(value, event)`; call sites expect the event, so the
 *   adapter hands the event back and nothing downstream needs to know.
 * - Astryx always renders a label. Ours is supplied by the surrounding markup,
 *   so the field's own label is visually hidden and named from `aria-label`
 *   (or, failing that, the placeholder) instead of showing a second caption.
 */
export type InputProps = React.ComponentProps<"input">;

/** The input types Astryx's TextInput renders; anything else falls back to text. */
const ASTRYX_TYPES = ["text", "password", "email", "search", "tel", "url"] as const;

function astryxType(type: string) {
  return (ASTRYX_TYPES as readonly string[]).includes(type)
    ? (type as (typeof ASTRYX_TYPES)[number])
    : ("text" as const);
}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  (
    {
      className,
      type = "text",
      id,
      value,
      defaultValue,
      onChange,
      disabled,
      readOnly,
      autoFocus,
      name,
      placeholder,
      onKeyDown,
      autoComplete,
      inputMode,
      "aria-label": ariaLabel,
      "aria-labelledby": ariaLabelledBy,
    },
    ref,
  ) => {
    // Astryx is value-driven. Call sites that still pass `defaultValue` get a
    // local value so the field behaves as uncontrolled, as it did before.
    const [draft, setDraft] = React.useState(defaultValue == null ? "" : String(defaultValue));
    const controlled = value !== undefined;
    const current = controlled ? String(value ?? "") : draft;

    return (
      <TextInput
        ref={ref}
        id={id}
        type={astryxType(type)}
        label={ariaLabelledBy ? "" : (ariaLabel ?? "")}
        isLabelHidden
        aria-label={ariaLabelledBy ? undefined : (ariaLabel ?? (placeholder || undefined))}
        aria-labelledby={ariaLabelledBy}
        value={current}
        onChange={(next, event) => {
          if (!controlled) setDraft(next);
          onChange?.(event);
        }}
        placeholder={placeholder}
        isDisabled={disabled}
        isReadOnly={readOnly}
        hasAutoFocus={autoFocus}
        htmlName={name}
        autoComplete={autoComplete}
        // Astryx has no `number` field (it ships NumberInput separately). Keeping
        // the numeric keypad is what the port / retention fields actually need;
        // the spinner arrows are the only loss, and nothing reads valueAsNumber.
        inputMode={inputMode ?? (type === "number" ? "numeric" : undefined)}
        onKeyDown={onKeyDown}
        width="100%"
        // Call sites style the field itself (widths, compact heights), which is
        // Astryx's `controlClassName`; `className` there would not typecheck.
        controlClassName={className}
      />
    );
  },
);
Input.displayName = "Input";

export { Input };
