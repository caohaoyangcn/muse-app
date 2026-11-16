import type { CardSize, ViewMode } from "../types";
import { DESKTOP_CARD_SIZE } from "../utils/viewPrefs";
import { SegmentedControl, SegmentedControlItem, Slider } from "../ui/astryx";
import { Icon } from "../ui/icon";

/**
 * The "how dense / which shape" half of the list toolbar: continuous card width
 * and the grid/list toggle. Astryx owns the slider and the segmented control;
 * the surrounding row only decides where they sit.
 */
export default function ViewControls({
  view,
  onViewChange,
  cardSize,
  onCardSizeChange,
}: {
  view: ViewMode;
  onViewChange: (view: ViewMode) => void;
  cardSize: CardSize;
  onCardSizeChange: (size: CardSize) => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <Slider
        isLabelHidden
        label="作品卡片大小"
        max={DESKTOP_CARD_SIZE.max}
        min={DESKTOP_CARD_SIZE.min}
        onChange={onCardSizeChange}
        step={1}
        value={cardSize}
        valueDisplay="none"
        width={120}
      />
      <SegmentedControl
        label="视图切换"
        layout="hug"
        onChange={(next) => onViewChange(next as ViewMode)}
        size="sm"
        value={view}
      >
        <SegmentedControlItem icon={<Icon className="size-3.5" name="grid" />} label="网格" value="grid" />
        <SegmentedControlItem icon={<Icon className="size-3.5" name="list" />} label="列表" value="list" />
      </SegmentedControl>
    </div>
  );
}
