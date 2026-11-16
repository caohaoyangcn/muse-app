import { useCallback, type ReactNode } from "react";
import { DropdownMenu } from "@astryxdesign/core/DropdownMenu";

/**
 * A point-anchored Astryx menu.
 *
 * The desktop context menu is opened at a cursor coordinate, while Astryx's
 * DropdownMenu is anchored to a trigger. This adapter renders a 1×1 invisible
 * trigger at that coordinate and opens the menu under it; Astryx then owns the
 * popover, viewport flipping, focus, outside-click/Escape dismissal and
 * keyboard navigation. Business files pass ordinary DropdownMenuItem /
 * DropdownMenuSubMenu children, so the menu content itself is Astryx too.
 */
export function MenuSurface({
  x,
  y,
  label,
  onClose,
  children,
}: {
  x: number;
  y: number;
  label: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) onClose();
    },
    [onClose],
  );

  return (
    <div
      className="fixed z-[200]"
      style={{ left: x, top: y, width: 1, height: 1 }}
    >
      <DropdownMenu
        isMenuOpen
        menuWidth={200}
        onOpenChange={handleOpenChange}
        placement="below"
        alignment="start"
        presentation="popover"
        renderTrigger={(props) => (
          <button
            {...props}
            aria-label={label}
            className="size-px cursor-default opacity-0"
            type="button"
          />
        )}
      >
        {children}
      </DropdownMenu>
    </div>
  );
}
