import * as React from "react";
import { cn } from "../lib/utils";
import { X } from "lucide-react";
import { Dialog as AstryxDialog } from "@astryxdesign/core/Dialog";

import { Button } from "./button";

/**
 * Modal dialog, on Astryx.
 *
 * Astryx owns the hard half — the native `<dialog>` with `showModal()`, the top
 * layer, the scrim, the focus trap, Escape/purpose handling and the entrance
 * motion. This module keeps the compound shape the app already uses
 * (`Dialog` + `DialogContent` + `DialogHeader`/`DialogTitle`/`Footer`) so the ten
 * dialogs read the same, and adds the two things Astryx leaves to the caller:
 * the close button and the dialog's accessible name.
 *
 * Width is a prop, not a class: Astryx writes its width as an inline custom
 * property, which a `w-[620px]` utility on the same element would not beat.
 */
type DialogContextValue = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  width?: number | string;
};

const DialogContext = React.createContext<DialogContextValue | null>(null);
const TitleIdContext = React.createContext<string | undefined>(undefined);

export function Dialog({
  open,
  onOpenChange,
  width,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Preferred surface width; numbers are pixels. */
  width?: number | string;
  children: React.ReactNode;
}) {
  const value = React.useMemo(() => ({ open, onOpenChange, width }), [open, onOpenChange, width]);
  return <DialogContext.Provider value={value}>{children}</DialogContext.Provider>;
}

export interface DialogContentProps extends React.HTMLAttributes<HTMLDivElement> {
  /** Accessible name when the dialog has no `DialogTitle`. */
  label?: string;
}

export const DialogContent = React.forwardRef<HTMLDivElement, DialogContentProps>(
  ({ className, children, label, ...props }, ref) => {
    const context = React.useContext(DialogContext);
    if (!context) throw new Error("DialogContent 必须在 Dialog 内使用");
    const titleId = React.useId();

    return (
      <AstryxDialog
        isOpen={context.open}
        onOpenChange={context.onOpenChange}
        width={context.width}
        // Call sites bring their own padding when they need edge-to-edge rows
        // (a header divider, a list, a footer bar); the token step is the default.
        padding={4}
        aria-labelledby={label ? undefined : titleId}
        aria-label={label}
        className={className}
      >
        <TitleIdContext.Provider value={titleId}>
          <div ref={ref} className="relative" {...props}>
            {children}
            <Button
              variant="ghost"
              size="sm"
              aria-label="关闭"
              className="absolute right-0 top-0 -m-1 text-[var(--muted)]"
              onClick={() => context.onOpenChange(false)}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        </TitleIdContext.Provider>
      </AstryxDialog>
    );
  },
);
DialogContent.displayName = "DialogContent";

export const DialogHeader = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
  <div className={cn("flex flex-col space-y-1.5", className)} {...props} />
);
DialogHeader.displayName = "DialogHeader";

export const DialogFooter = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
  <div className={cn("flex justify-end gap-2", className)} {...props} />
);
DialogFooter.displayName = "DialogFooter";

export const DialogTitle = React.forwardRef<HTMLHeadingElement, React.HTMLAttributes<HTMLHeadingElement>>(
  ({ className, ...props }, ref) => {
    const titleId = React.useContext(TitleIdContext);
    return (
      <h2
        ref={ref}
        id={titleId}
        className={cn("text-base font-semibold leading-none", className)}
        {...props}
      />
    );
  },
);
DialogTitle.displayName = "DialogTitle";

export const DialogDescription = React.forwardRef<
  HTMLParagraphElement,
  React.HTMLAttributes<HTMLParagraphElement>
>(({ className, ...props }, ref) => (
  <p
    ref={ref}
    className={cn("text-sm leading-relaxed text-[var(--muted)]", className)}
    {...props}
  />
));
DialogDescription.displayName = "DialogDescription";
