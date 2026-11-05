import { Button } from "./ui/button";

export default function EmptyState({
  keyword,
  onAdd,
}: {
  keyword: string;
  onAdd: () => void;
}) {
  if (keyword.trim()) {
    return (
      <div className="flex h-full min-h-[320px] flex-col items-center justify-center text-center text-[var(--muted)]">
        <div className="mb-1.5 text-xl text-[var(--text)]">无结果</div>
        <div>没有匹配“{keyword.trim()}”的内容</div>
      </div>
    );
  }
  return (
    <div className="flex h-full min-h-[320px] flex-col items-center justify-center text-center text-[var(--muted)]">
      <div className="mb-1.5 text-xl text-[var(--text)]">暂无收藏视频</div>
      <div className="mb-5">添加你喜欢的视频</div>
      <Button variant="primary" size="lg" onClick={onAdd}>
        + 添加视频
      </Button>
      <div className="mt-3.5 text-xs">…或将视频文件拖入窗口任意位置</div>
    </div>
  );
}
