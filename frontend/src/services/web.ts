// 局域网移动端门面（web）：壳只从这里取数，不直接 import 传输模块。
//
// `client` 覆盖两端都有的能力（实现见 client.http.ts）。这里直出的是**浏览器能做、
// 但桌面端用另一套实现**的东西：媒体 URL 拼装、按 id 取单条收藏、播放记账，以及
// 形状与桌面端不同的 `videosInGroup`（服务端直接返回 Video[]，桌面端返回 id 列表）。
// 错误类型 `ApiError` 也在此，壳用它判断"服务端给了可展示的文案"。
export { client } from "./client";
export { ApiError, coverURL, getVideo, markVideoOpened, videosInGroup } from "./http";
