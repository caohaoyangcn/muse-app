import type { Video } from "../../types";
import { Badge, List, ListItem } from "../../ui/astryx";
import { BottomSheet } from "../../ui/sheet";
import { externalPlayers, type ExternalPlayerID } from "../externalPlayers";

type Props = {
  video: Video | null;
  open: boolean;
  preferred: ExternalPlayerID | null;
  notice?: string;
  onChoose: (id: ExternalPlayerID) => void;
  onCopy: () => void;
  onClose: () => void;
};

/** Chooser/fallback sheet for handing a favorite to an iOS-native player. */
export function ExternalPlayerSheet({ video, open, preferred, notice, onChoose, onCopy, onClose }: Props) {
  return (
    <BottomSheet onClose={onClose} open={open && !!video} title="选择播放器">
      <p className="px-3 pb-2 text-xs leading-relaxed text-[var(--muted)]">
        {notice || "选择后将记住默认播放器。浏览器无法读取已安装应用列表，此入口会尝试直接打开对应客户端。"}
      </p>
      <List className="flex flex-col">
        {externalPlayers.map((player) => (
          <ListItem
            endContent={preferred === player.id ? <Badge label="默认" variant="info" /> : undefined}
            key={player.id}
            label={player.name}
            description={player.description}
            onClick={() => onChoose(player.id)}
          />
        ))}
        <ListItem label="复制播放地址" onClick={onCopy} />
      </List>
      <div className="flex flex-wrap gap-x-3 gap-y-1 px-3 pb-2 text-xs text-[var(--muted)]">
        {externalPlayers.map((player) => (
          <a
            className="underline underline-offset-2"
            href={player.appStoreURL}
            key={player.id}
            rel="noreferrer"
            target="_blank"
          >
            {player.name} App Store
          </a>
        ))}
      </div>
    </BottomSheet>
  );
}
