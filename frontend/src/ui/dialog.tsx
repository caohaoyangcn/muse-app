import * as React from "react";
import { cn } from "../lib/utils";
import { X } from "lucide-react";
import {
  Dialog as AstryxDialog,
  DialogHeader as AstryxDialogHeader,
  type DialogHeaderProps as AstryxDialogHeaderProps,
} from "@astryxdesign/core/Dialog";

import { IconButton } from "./astryx";

/**
 * Modal dialog, on Astryx.
 *
 * Astryx owns the hard half — the native `<dialog>` with `showModal()`, the top
 * layer, the scrim, the focus trap, Escape/purpose handling and the entrance
 * motion. This module only keeps the compound shape the app already uses
 * (`Dialog` + `DialogContent` + `DialogHeader`/`Footer`).
 *
 * The header is Astryx's `DialogHeader`, not a heading we draw ourselves: it
 * autofocuses the title, names the dialog through `aria-labelledby`, and owns
 * the close button (with its tooltip). A dialog that has no header — an image
 * lightbox — can render `DialogCloseButton` instead.
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

function useDialogContext(component: string) {
  const context = React.useContext(DialogContext);
  if (!context) throw new Error(`${component} 必须在 Dialog 内使用`);
  return context;
}

export function Dialog({
  open,
  onOpenChange,
  width,
  label,
  purpose = "info",
  padding = 4,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Preferred surface width; numbers are pixels. */
  width?: number | string;
  /** Accessible name for a dialog that has no `DialogHeader`. */
  label?: string;
  /** Dismissal policy: `form` keeps a stray backdrop click from throwing away
   *  typed content, `required` also blocks Escape. */
  purpose?: "required" | "form" | "info";
  /** Internal padding step. Pass `0` for edge-to-edge rows that bring their own. */
  padding?: 0 | 0.5 | 1 | 1.5 | 2 | 3 | 4 | 5 | 6 | 8 | 10;
  children: React.ReactNode;
}) {
  return (
    <DialogContext.Provider value={{ open, onOpenChange, width }}>
      <AstryxDialog
        isOpen={open}
        onOpenChange={onOpenChange}
        width={width}
        aria-label={label}
        purpose={purpose}
        padding={padding}
      >
        {children}
      </AstryxDialog>
    </DialogContext.Provider>
  );
}

export type DialogContentProps = React.HTMLAttributes<HTMLDivElement>;

export const DialogContent = React.forwardRef<HTMLDivElement, DialogContentProps>(
  ({ className, children, ...props }, ref) => (
    <div ref={ref} className={cn("relative", className)} {...props}>
      {children}
    </div>
  ),
);
DialogContent.displayName = "DialogContent";

export type DialogHeaderProps = Omit<AstryxDialogHeaderProps, "onOpenChange"> & {
  onOpenChange?: (isOpen: boolean) => unknown;
};

/**
 * The canonical Astryx dialog header. Omitting `onOpenChange` uses the dialog's
 * own handler, so every header gets the standard close button.
 */
export function DialogHeader({ onOpenChange, ...props }: DialogHeaderProps) {
  const context = useDialogContext("DialogHeader");
  return (
    <AstryxDialogHeader
      onOpenChange={onOpenChange ?? context.onOpenChange}
      {...props}
    />
  );
}

/** A floating close control for headerless dialogs (image lightboxes). */
export function DialogCloseButton({ className }: { className?: string }) {
  const context = useDialogContext("DialogCloseButton");
  return (
    <IconButton
      className={cn("absolute right-2 top-2 z-10", className)}
      icon={<X className="h-4 w-4" />}
      label="关闭"
      onClick={() => context.onOpenChange(false)}
      size="sm"
      tooltip="关闭"
      variant="ghost"
    />
  );
}

export const DialogFooter = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
  <div className={cn("flex justify-end gap-2", className)} {...props} />
);
DialogFooter.displayName = "DialogFooter";
