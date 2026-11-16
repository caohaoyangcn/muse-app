import type { HTMLAttributes } from "react";
import alert from "../assets/icons/nav/alert.svg?raw";
import archive from "../assets/icons/nav/archive.svg?raw";
import arrowLeft from "../assets/icons/nav/arrow-left.svg?raw";
import check from "../assets/icons/nav/check.svg?raw";
import chevronDown from "../assets/icons/nav/chevron-down.svg?raw";
import chevronRight from "../assets/icons/nav/chevron-right.svg?raw";
import duplicate from "../assets/icons/nav/duplicate.svg?raw";
import filter from "../assets/icons/nav/filter.svg?raw";
import folder from "../assets/icons/nav/folder.svg?raw";
import folderOpen from "../assets/icons/nav/folder-open.svg?raw";
import grid from "../assets/icons/nav/grid.svg?raw";
import hardDrive from "../assets/icons/nav/hard-drive.svg?raw";
import heart from "../assets/icons/nav/heart.svg?raw";
import history from "../assets/icons/nav/history.svg?raw";
import list from "../assets/icons/nav/list.svg?raw";
import listChecks from "../assets/icons/nav/list-checks.svg?raw";
import menu from "../assets/icons/nav/menu.svg?raw";
import moreHorizontal from "../assets/icons/nav/more-horizontal.svg?raw";
import plus from "../assets/icons/nav/plus.svg?raw";
import star from "../assets/icons/nav/star.svg?raw";
import users from "../assets/icons/nav/users.svg?raw";
import x from "../assets/icons/nav/x.svg?raw";
import { cn } from "../lib/utils";

/**
 * The app's own glyphs. The SVGs under `assets/icons/nav/` are the editable
 * source: Vite's `?raw` import inlines them, so a single
 * `stroke="currentColor"` follows the surrounding text or accent color at any
 * size without a separate icon build step.
 *
 * This lives in `ui/` because both shells draw the same vocabulary: the desktop
 * sidebar and the phone's navigation, filters and card actions share one set, so
 * a new affordance looks the same in both without a second copy.
 *
 * The markup is first-party static content, never user input; inlining it with
 * `dangerouslySetInnerHTML` is what keeps the SVG file itself the source of
 * truth for future design edits.
 */
const ICONS = {
  alert,
  archive,
  "arrow-left": arrowLeft,
  check,
  "chevron-down": chevronDown,
  "chevron-right": chevronRight,
  duplicate,
  filter,
  folder,
  "folder-open": folderOpen,
  grid,
  "hard-drive": hardDrive,
  heart,
  history,
  list,
  "list-checks": listChecks,
  menu,
  "more-horizontal": moreHorizontal,
  plus,
  star,
  users,
  x,
} as const;

export type NavIconName = keyof typeof ICONS;

export function NavIcon({
  name,
  className,
  ...props
}: { name: NavIconName } & Omit<
  HTMLAttributes<HTMLSpanElement>,
  "children" | "dangerouslySetInnerHTML"
>) {
  return (
    <span
      aria-hidden="true"
      {...props}
      className={cn(
        "inline-flex h-4 w-4 shrink-0 [&>svg]:block [&>svg]:h-full [&>svg]:w-full",
        className,
      )}
      dangerouslySetInnerHTML={{ __html: ICONS[name] }}
    />
  );
}
