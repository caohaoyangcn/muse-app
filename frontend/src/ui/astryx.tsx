/**
 * The design-system components used by Muse's business views.
 *
 * This is the only import surface for these Astryx primitives; business files
 * still never import `@astryxdesign/*` directly. Keeping the re-exports here
 * (instead of one wrapper per component) is deliberate: the components already
 * have stable, documented props, and a wrapper would only rename them.
 */

export { AlertDialog } from "@astryxdesign/core/AlertDialog";
export { AspectRatio } from "@astryxdesign/core/AspectRatio";
export { Avatar } from "@astryxdesign/core/Avatar";
export { Badge } from "@astryxdesign/core/Badge";
export { Banner } from "@astryxdesign/core/Banner";
export { ButtonGroup } from "@astryxdesign/core/ButtonGroup";
export { CheckboxInput } from "@astryxdesign/core/CheckboxInput";
export { CheckboxList, CheckboxListItem } from "@astryxdesign/core/CheckboxList";
export { ClickableCard } from "@astryxdesign/core/ClickableCard";
export { Divider } from "@astryxdesign/core/Divider";
export {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuDivider,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSubMenu,
} from "@astryxdesign/core/DropdownMenu";
export { EmptyState } from "@astryxdesign/core/EmptyState";
export { Field } from "@astryxdesign/core/Field";
export { FormLayout } from "@astryxdesign/core/FormLayout";
export { Grid } from "@astryxdesign/core/Grid";
export { HoverCard } from "@astryxdesign/core/HoverCard";
export { IconButton } from "@astryxdesign/core/IconButton";
export { List, ListItem } from "@astryxdesign/core/List";
export { MetadataList, MetadataListItem } from "@astryxdesign/core/MetadataList";
export { MoreMenu } from "@astryxdesign/core/MoreMenu";
export { ProgressBar } from "@astryxdesign/core/ProgressBar";
export { RadioList, RadioListItem } from "@astryxdesign/core/RadioList";
export { Section } from "@astryxdesign/core/Section";
export { Selector } from "@astryxdesign/core/Selector";
export { SegmentedControl, SegmentedControlItem } from "@astryxdesign/core/SegmentedControl";
export { Skeleton } from "@astryxdesign/core/Skeleton";
export { Slider } from "@astryxdesign/core/Slider";
export { Spinner } from "@astryxdesign/core/Spinner";
export { Tab, TabList } from "@astryxdesign/core/TabList";
export { TextArea } from "@astryxdesign/core/TextArea";
export { ToggleButton, ToggleButtonGroup } from "@astryxdesign/core/ToggleButton";
export { Toolbar } from "@astryxdesign/core/Toolbar";

/**
 * The shared look for a multi-select chip in a ToggleButtonGroup. Astryx's
 * ToggleButton is ghost by default; the filter chips need a resting surface so
 * an unselected condition is still a visible target.
 */
export const toggleChipClass =
  "rounded-full border border-[var(--border)] bg-[var(--panel-2)] px-3 text-xs text-[var(--text)] data-[is-pressed=true]:border-transparent data-[is-pressed=true]:bg-[var(--accent)] data-[is-pressed=true]:text-white";

/** Compact badge height for rows whose vertical rhythm is fixed (cards and
 *  virtualized lists). Keeps an occasional 已备份/破解 marker from changing the
 *  measured row height. */
export const compactBadgeClass = "h-4 shrink-0 px-1.5 py-0 text-[10px] leading-none";
