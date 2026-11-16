import { AlertDialog } from "../ui/astryx";

interface ConfirmDialogProps {
  title: string;
  message: string;
  confirmLabel: string;
  cancelLabel: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

/**
 * Astryx's AlertDialog, not a Dialog with two buttons: the component owns the
 * alertdialog role, the title/description wiring, the blocked backdrop and the
 * focus order (Cancel is the least destructive target).
 */
export default function ConfirmDialog({
  title,
  message,
  confirmLabel,
  cancelLabel,
  danger,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  return (
    <AlertDialog
      isOpen
      actionLabel={confirmLabel}
      actionVariant={danger ? "destructive" : "primary"}
      cancelLabel={cancelLabel}
      description={message}
      onAction={onConfirm}
      onOpenChange={(open) => !open && onCancel()}
      title={title}
    />
  );
}
