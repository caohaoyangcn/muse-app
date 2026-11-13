import type { Video } from "../../types";
import {
  externalPlayers,
  type ExternalPlayerID,
} from "../externalPlayers";
import { BottomSheet, SheetAction } from "../../ui/sheet";

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
      {externalPlayers.map((player) => (
        <div key={player.id}>
          <SheetAction
            label={player.name}
            hint={preferred === player.id ? "默认" : player.description}
            onClick={() => onChoose(player.id)}
          />
          <div className="-mt-1 px-3 pb-2 text-xs text-[var(--muted)]">
            <a className="underline underline-offset-2" href={player.appStoreURL} rel="noreferrer" target="_blank">
              未安装？前往 App Store
            </a>
          </div>
        </div>
      ))}
      <SheetAction label="复制播放地址" onClick={onCopy} />
    </BottomSheet>
  );
}
