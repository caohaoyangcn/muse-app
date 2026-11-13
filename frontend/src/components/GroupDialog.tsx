import { useEffect, useRef, useState } from "react";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";

interface GroupDialogProps {
  title: string;
  initialName?: string;
  confirmLabel: string;
  cancelLabel: string;
  placeholder?: string;
  onConfirm: (name: string) => void;
  onCancel: () => void;
}

/** Modal with a single text field for creating or renaming a group. */
export default function GroupDialog({
  title,
  initialName = "",
  confirmLabel,
  cancelLabel,
  placeholder = "分组名称",
  onConfirm,
  onCancel,
}: GroupDialogProps) {
  const [name, setName] = useState(initialName);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  const submit = () => {
    const trimmed = name.trim();
    if (trimmed) onConfirm(trimmed);
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onCancel()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription className="sr-only">请输入分组名称</DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
          className="space-y-4"
        >
          <Input
          ref={inputRef}
            value={name}
            placeholder={placeholder}
            spellCheck={false}
            onChange={(e) => setName(e.target.value)}
          />
          <DialogFooter>
            <Button type="button" onClick={onCancel}>
              {cancelLabel}
            </Button>
            <Button type="submit" variant="primary" disabled={!name.trim()}>
              {confirmLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
