export default function DropOverlay() {
  return (
    <div className="pointer-events-none fixed inset-0 z-[100] flex items-center justify-center bg-[rgba(23,23,27,0.85)]">
      <div className="rounded-2xl border-2 border-dashed border-[var(--accent)] bg-[rgba(124,92,255,0.08)] px-[72px] py-12 text-center">
        <div className="text-[22px] font-semibold">将视频拖到此处</div>
        <div className="mt-2 text-[var(--muted)]">+ 添加收藏</div>
      </div>
    </div>
  );
}
