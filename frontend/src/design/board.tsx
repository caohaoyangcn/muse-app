// Design-system sheet for the Figma handoff.
//
// This page exists so a component library can be imported into Figma from one
// URL instead of being rebuilt by hand: every section names the token or the
// `ui/` module it comes from, and nothing here is a one-off style. It is served
// by the design preview (`npm run dev:design`) at /src/design/board.html and is
// never part of either production bundle.
import type { ReactNode } from "react";
import { Badge, EmptyState, MetadataList, MetadataListItem, Spinner } from "../ui/astryx";
import { Button } from "../ui/button";
import { CoverImage } from "../ui/cover";
import { Icon } from "../ui/icon";
import { Input } from "../ui/input";
import { Pagination } from "../ui/pagination";
import { ReportBanner } from "../ui/report";

const COLORS = [
  ["--bg", "#17171b", "页面底色"],
  ["--panel", "#1d1d23", "面板 / 弹层"],
  ["--panel-2", "#232329", "面板内层"],
  ["--card", "#26262d", "卡片"],
  ["--border", "#32323a", "描边"],
  ["--text", "#e9e9ec", "主文字"],
  ["--muted", "#9b9ba5", "次要文字"],
  ["--accent", "#7c5cff", "强调 / 选中"],
  ["--accent-hover", "#8f74ff", "强调悬停"],
  ["--danger", "#e5534b", "危险操作"],
];

const TYPE_SCALE = [
  ["10px", "徽标、卡片角标", "compactBadgeClass"],
  ["11px", "计数、辅助信息", "text-[11px]"],
  ["13px", "正文与导航（默认）", "text-[13px]"],
  ["15px", "工具栏标题", "text-[15px]"],
  ["17px", "页面标题", "text-[17px]"],
  ["20px", "详情标题", "text-xl"],
  ["22px", "空状态标题", "text-[22px]"],
];

const SPACES = [4, 8, 12, 16, 20, 24, 32];

function Section({ title, note, children }: { title: string; note: string; children: ReactNode }) {
  return (
    <section className="border-t border-[var(--border)] px-10 py-8">
      <div className="mb-5 flex items-baseline gap-3">
        <h2 className="m-0 text-[15px] font-semibold">{title}</h2>
        <span className="text-[11px] text-[var(--muted)]">{note}</span>
      </div>
      {children}
    </section>
  );
}

function Label({ children }: { children: ReactNode }) {
  return <div className="mb-2 text-[11px] text-[var(--muted)]">{children}</div>;
}

export default function Board() {
  return (
    <div className="min-h-screen bg-[var(--bg)] pb-20 text-[var(--text)]">
      <header className="px-10 pb-6 pt-10">
        <div>
          <h1 className="m-0 text-2xl font-semibold tracking-[0.2px]">Muse Video · 设计基元</h1>
          <p className="mb-0 mt-2 max-w-[720px] text-[13px] leading-6 text-[var(--muted)]">
            这一页由真实组件渲染，导入 Figma 后即可得到与代码同源的组件库。
            每个分组下的灰色注释是它在源码里的位置：token 一律来自{" "}
            <code className="text-[var(--text)]">frontend/src/theme.css</code>，组件一律来自{" "}
            <code className="text-[var(--text)]">frontend/src/ui/</code>。
          </p>
        </div>
      </header>

      <Section title="Color" note="theme.css:22 · Figma Variables / Collection「Muse / Dark」">
        <div className="grid grid-cols-5 gap-4">
          {COLORS.map(([token, hex, role]) => (
            <div key={token} className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)]">
              <div className="h-20" style={{ background: hex }} />
              <div className="px-3 py-2">
                <div className="font-mono text-[11px]">{token}</div>
                <div className="mt-0.5 flex items-center justify-between text-[10px] text-[var(--muted)]">
                  <span>{hex}</span>
                  <span>{role}</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      </Section>

      <Section title="Typography" note="Sarasa UI SC · 字号按实际使用频次收敛，无 24px 以上展示体">
        <div className="space-y-3">
          {TYPE_SCALE.map(([size, usage, source]) => (
            <div key={size} className="flex items-baseline gap-6">
              <span className="w-14 shrink-0 font-mono text-[11px] text-[var(--muted)]">{size}</span>
              <span style={{ fontSize: size }} className="shrink-0 font-semibold">
                盛夏海边，与你
              </span>
              <span style={{ fontSize: size }} className="shrink-0">
                MUSE-101 · 2:28:00
              </span>
              <span className="ml-auto font-mono text-[10px] text-[var(--muted)]">
                {usage} · {source}
              </span>
            </div>
          ))}
        </div>
      </Section>

      <Section title="Spacing" note="4px 基准；卡片内边距 12，区块间距 20–24">
        <div className="flex items-end gap-4">
          {SPACES.map((space) => (
            <div key={space} className="flex flex-col items-center gap-2">
              <div
                className="rounded-sm bg-[var(--accent)] opacity-70"
                style={{ width: space * 2, height: space * 2 }}
              />
              <span className="font-mono text-[10px] text-[var(--muted)]">{space}</span>
            </div>
          ))}
        </div>
      </Section>

      <Section title="Radius" note="--radius: 10px；元素取 var(--radius)，容器 +2">
        <div className="flex gap-4">
          {[
            ["5px", "徽标 / 角标"],
            ["6px", "输入框内元素"],
            ["10px", "--radius 元素"],
            ["12px", "--radius-container"],
            ["999px", "胶囊（chip / 计数）"],
          ].map(([radius, usage]) => (
            <div key={radius} className="w-[150px]">
              <div
                className="h-16 border border-[var(--border)] bg-[var(--panel-2)]"
                style={{ borderRadius: radius }}
              />
              <div className="mt-2 font-mono text-[11px]">{radius}</div>
              <div className="text-[10px] text-[var(--muted)]">{usage}</div>
            </div>
          ))}
        </div>
      </Section>

      <Section title="Button" note="ui/button.tsx · variant × size，映射到 Astryx secondary/primary/ghost/destructive">
        <div className="space-y-5">
          <div>
            <Label>variant</Label>
            <div className="flex items-center gap-3">
              <Button variant="primary">+ 添加视频</Button>
              <Button>筛选</Button>
              <Button variant="ghost">取消</Button>
              <Button variant="danger">移入回收站</Button>
              <Button disabled>不可用</Button>
            </div>
          </div>
          <div>
            <Label>size（sm / 默认 / lg）</Label>
            <div className="flex items-center gap-3">
              <Button size="sm">小按钮</Button>
              <Button>默认按钮</Button>
              <Button size="lg">大按钮</Button>
              <Button icon={<Icon name="plus" />} variant="primary">
                带图标
              </Button>
            </div>
          </div>
          <div>
            <Label>IconButton（纯图标，必须带无障碍名称）</Label>
            <div className="flex items-center gap-2">
              <Button className="w-8 px-0" variant="ghost" aria-label="后退">
                ‹
              </Button>
              <Button className="w-8 px-0" variant="ghost" aria-label="前进">
                ›
              </Button>
              <Button variant="ghost" icon={<Icon name="star" />} aria-label="收藏" />
              <Button variant="ghost" icon={<Icon name="filter" />} aria-label="筛选" />
            </div>
          </div>
        </div>
      </Section>

      <Section title="Input" note="ui/input.tsx · 输入框高度 32，描边用 --border，聚焦用 --accent">
        <div className="grid max-w-[860px] gap-4">
          <div>
            <Label>空状态</Label>
            <Input placeholder="搜索视频、番号或演员" aria-label="搜索" />
          </div>
          <div>
            <Label>已填写 · 带可见标签</Label>
            <Input label="WebDAV 地址" defaultValue="https://nas.local/dav/muse" />
          </div>
          <div>
            <Label>数字输入</Label>
            <Input label="端口" defaultValue="8899" inputMode="numeric" />
          </div>
        </div>
      </Section>

      <Section title="Badge / Chip" note="ui/astryx.tsx 的 compactBadgeClass 与 toggleChipClass">
        <div className="flex flex-wrap items-center gap-3">
          <Badge label="已备份" variant="success" />
          <Badge label="马赛克破解" variant="purple" />
          <Badge label="文件缺失" variant="error" />
          <Badge label="内容已变化" variant="warning" />
          <Badge label="4K" variant="neutral" />
          <Badge className="h-4 shrink-0 px-1.5 py-0 text-[10px] leading-none" label="紧凑徽标" variant="success" />
          <span className="inline-flex h-6 items-center rounded-full border border-[rgba(124,92,255,0.35)] bg-[rgba(124,92,255,0.12)] px-3 text-[11px] text-[var(--text)]">
            已选 3 部
          </span>
        </div>
        <div className="mt-4 flex flex-wrap items-center gap-2">
          <Button
            className="rounded-full border border-[var(--border)] bg-[var(--panel-2)] px-3 text-xs text-[var(--text)]"
            data-is-pressed="true"
          >
            已选 chip
          </Button>
          <Button className="rounded-full border border-[var(--border)] bg-[var(--panel-2)] px-3 text-xs text-[var(--text)]">
            未选 chip
          </Button>
          <Button
            className="rounded-full border border-[var(--border)] bg-[var(--panel-2)] px-3 text-xs text-[var(--text)]"
            icon={<Icon className="size-3" name="arrow-up" />}
          >
            排序 chip
          </Button>
        </div>
      </Section>

      <Section title="Cover" note="ui/cover.tsx · portrait 右裁只露封面；landscape 等比收进完整封面+封底">
        <div className="flex items-start gap-6">
          <div>
            <Label>portrait · 3:4（详情 / 演员作品）</Label>
            <CoverImage
              className="h-[240px] w-[180px] rounded-[var(--radius)] bg-[#111114]"
              src="/thumbnails/1.jpg"
              variant="portrait"
            />
          </div>
          <div>
            <Label>landscape · 16:9（卡片 / 列表行）</Label>
            <CoverImage
              className="h-[180px] w-[320px] rounded-[var(--radius)] bg-[#111114]"
              src="/thumbnails/2.jpg"
              variant="landscape"
            />
          </div>
          <div className="max-w-[320px]">
            <Label>卡片组合（封面 + 标题 + 元信息 + 点赞）</Label>
            <div className="overflow-hidden rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)]">
              <CoverImage className="block aspect-video w-full" src="/thumbnails/3.jpg" variant="landscape" />
              <div className="px-3 py-2">
                <div className="text-[13px] font-medium">【MUSE-101】盛夏海边，与你</div>
                <div className="mt-1 flex items-center gap-2 text-[11px] text-[var(--muted)]">
                  <span>2:28:00</span>
                  <span>1920×1080</span>
                  <span>6.4 GB</span>
                  <span className="ml-auto inline-flex items-center gap-1">
                    <Icon className="size-3" name="heart" />
                    12
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </Section>

      <Section title="Pagination" note="ui/pagination.tsx · 首尾 + 当前页前后各 5 页，跳空用 …">
        <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)]">
          <Pagination page={7} pageCount={24} total={215} unit="部" onPageChange={() => {}} />
        </div>
      </Section>

      <Section title="Feedback" note="ReportBanner（长结果）/ EmptyState / Spinner / 计数">
        <div className="space-y-4">
          <ReportBanner
            reports={[
              {
                id: 1,
                kind: "success",
                message: "已完成 12 部刮削",
                details: ["成功 10 部", "跳过 1 部（未识别番号）", "失败 1 部（网络超时）"],
              },
            ]}
            onDismiss={() => {}}
          />
          <div className="grid grid-cols-2 gap-4">
            <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)]">
              <EmptyState
                description="播放过的视频会按 1 天、3 天、7 天、14 天分组出现在这里"
                title="还没有播放记录"
              />
            </div>
            <div className="flex items-center justify-center rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel)]">
              <Spinner label="加载中…" size="lg" />
            </div>
          </div>
        </div>
      </Section>

      <Section title="Metadata" note="MetadataList · 详情页两列瀑布，标签列用 12px 键名 + 13px 值">
        <MetadataList columns={2} label={{ position: "top" }}>
          <MetadataListItem label="番号">MUSE-101</MetadataListItem>
          <MetadataListItem label="标题">真夏の海辺で、君と</MetadataListItem>
          <MetadataListItem label="中文译名">盛夏海边，与你</MetadataListItem>
          <MetadataListItem label="制作商">MUSE Works</MetadataListItem>
          <MetadataListItem label="系列">MUSE 系列</MetadataListItem>
          <MetadataListItem label="发行日期">2025年2月11日</MetadataListItem>
          <MetadataListItem label="时长">148 分钟</MetadataListItem>
          <MetadataListItem label="是否无码">否</MetadataListItem>
        </MetadataList>
      </Section>
    </div>
  );
}
