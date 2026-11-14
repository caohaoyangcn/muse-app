import * as React from "react";
import { Button as AstryxButton } from "@astryxdesign/core/Button";

import { cn } from "../lib/utils";

/**
 * The one button in the app, on Astryx.
 *
 * The props are the Moss-era ones on purpose — `variant="primary|danger|ghost"`,
 * `size`, plain `<button>` attributes, children as the visible content — so the
 * ~20 call sites keep reading naturally and only this file knows the design
 * system's vocabulary. The mapping is one-way:
 *
 *   default → secondary    primary → primary
 *   danger  → destructive  ghost   → ghost
 *
 * Text goes through `children`, not Astryx's `label`: call sites mix text with
 * icons and counts, which a `label: string` cannot express. The accessible name
 * is derived from those children (see `textOf`) and passed as `aria-label`, so a
 * button with an icon and a word is still announced as that word.
 */
const VARIANTS = {
  default: "secondary",
  primary: "primary",
  danger: "destructive",
  ghost: "ghost",
} as const;

const SIZES = {
  default: "md",
  sm: "sm",
  lg: "lg",
} as const;

export type ButtonVariant = keyof typeof VARIANTS;
export type ButtonSize = keyof typeof SIZES;

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
}

/** Visible text of a children tree, ignoring elements that carry no words. */
function textOf(node: React.ReactNode): string {
  if (node == null || typeof node === "boolean") return "";
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(textOf).join(" ");
  if (React.isValidElement<{ children?: React.ReactNode }>(node)) return textOf(node.props.children);
  return "";
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = "default", size = "default", type = "button", children, ...props }, ref) => {
    // Astryx takes the accessible name as `label` and the visible content as
    // `children`; when the two differ it emits `aria-label` itself. Passing both
    // is therefore the whole adapter: icon+text rows keep their words, and a
    // text-only button still reads as its text.
    const text = props["aria-label"] ?? textOf(children);
    const { disabled, ...rest } = props;

    return (
      <AstryxButton
        ref={ref}
        type={type}
        label={text}
        variant={VARIANTS[variant]}
        size={SIZES[size]}
        isDisabled={disabled}
        className={cn("muse-button", className)}
        {...rest}
      >
        {children}
      </AstryxButton>
    );
  },
);
Button.displayName = "Button";

export { Button };
