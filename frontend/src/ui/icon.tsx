import type { SVGProps } from "react";
import { Icon as AstryxIcon, registerIcons, type IconSize } from "@astryxdesign/core/Icon";

import { cn } from "../lib/utils";
import {
  ArchiveIcon,
  DuplicateIcon,
  FolderIcon,
  FolderOpenIcon,
  GridIcon,
  HardDriveIcon,
  HeartIcon,
  HistoryIcon,
  ListChecksIcon,
  ListIcon,
  PlusIcon,
  StarIcon,
  TagIcon,
  UsersIcon,
} from "./muse-icons";

/**
 * Muse's icon vocabulary, rendered through Astryx's `Icon`.
 *
 * Astryx owns the size/colour/accessibility contract; the built-in semantic
 * names cover the glyphs it already ships (menu, chevrons, close, warning,
 * funnel, …). Muse's own artwork is registered under `muse:*` keys so a theme
 * could still override it later, and this module keeps the kebab-case names the
 * business files already use.
 */
registerIcons({
  "muse:archive": <ArchiveIcon />,
  "muse:duplicate": <DuplicateIcon />,
  "muse:folder": <FolderIcon />,
  "muse:folder-open": <FolderOpenIcon />,
  "muse:grid": <GridIcon />,
  "muse:hard-drive": <HardDriveIcon />,
  "muse:heart": <HeartIcon />,
  "muse:history": <HistoryIcon />,
  "muse:list": <ListIcon />,
  "muse:list-checks": <ListChecksIcon />,
  "muse:plus": <PlusIcon />,
  "muse:star": <StarIcon />,
  "muse:tag": <TagIcon />,
  "muse:users": <UsersIcon />,
});

const BUILTIN = {
  alert: "warning",
  "arrow-left": "chevronLeft",
  "arrow-up": "arrowUp",
  "arrow-down": "arrowDown",
  check: "check",
  "chevron-down": "chevronDown",
  "chevron-right": "chevronRight",
  filter: "funnel",
  menu: "menu",
  "more-horizontal": "moreHorizontal",
  search: "search",
  x: "close",
} as const;

export type MuseIconName =
  | keyof typeof BUILTIN
  | "archive"
  | "duplicate"
  | "folder"
  | "folder-open"
  | "grid"
  | "hard-drive"
  | "heart"
  | "history"
  | "list"
  | "list-checks"
  | "plus"
  | "star"
  | "tag"
  | "users";

export function Icon({
  name,
  className,
  label,
  size = "sm",
  ...props
}: {
  name: MuseIconName;
  className?: string;
  label?: string;
  size?: IconSize;
} & Omit<SVGProps<SVGSVGElement>, "name" | "ref" | "color">) {
  const builtin = BUILTIN[name as keyof typeof BUILTIN];
  return (
    <AstryxIcon
      className={cn("shrink-0", className)}
      icon={builtin ?? `muse:${name}`}
      label={label}
      size={size}
      {...props}
    />
  );
}
