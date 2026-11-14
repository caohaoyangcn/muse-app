// Unit-tests the extension's address logic without a browser. config.js only
// touches chrome.* inside async loaders, so the pure helpers are importable.
//
//   node scripts/check-extension-config.mjs
//
// The point of these helpers is that a user-typed host/port can never become
// anything but an http:// loopback URL: a bad port, a remote host or a
// javascript: string must all fall back to the default instead of leaking into
// a fetch() or a tab URL.
import {
  buildBaseURL,
  DEFAULT_HOST,
  DEFAULT_PORT,
  isLoopbackHost,
  originPattern,
  parseBaseURL,
  parsePort,
} from "../browser-extension/config.js";

let failures = 0;
const eq = (label, got, want) => {
  const ok = JSON.stringify(got) === JSON.stringify(want);
  if (!ok) failures++;
  console.log(`${ok ? "ok  " : "FAIL"} ${label} → ${JSON.stringify(got)}${ok ? "" : ` (want ${JSON.stringify(want)})`}`);
};

console.log("-- parsePort --");
for (const [input, want] of [
  ["8899", 8899],
  [" 9123 ", 9123],
  ["0", 0],
  ["65536", 0],
  ["-1", 0],
  ["abc", 0],
  ["", 0],
  ["8899.5", 8899],
]) eq(`parsePort(${JSON.stringify(input)})`, parsePort(input), want);

console.log("\n-- isLoopbackHost --");
for (const [input, want] of [
  ["127.0.0.1", true],
  ["127.0.0.1", true],
  ["localhost", true],
  ["LOCALHOST", true],
  ["::1", true],
  ["192.168.1.5", false],
  ["evil.example", false],
  ["", false],
]) eq(`isLoopbackHost(${JSON.stringify(input)})`, isLoopbackHost(input), want);

console.log("\n-- buildBaseURL --");
eq("custom port", buildBaseURL("127.0.0.1", 9123), "http://127.0.0.1:9123");
eq("defaults", buildBaseURL(undefined, undefined), `http://${DEFAULT_HOST}:${DEFAULT_PORT}`);
eq("bad port falls back", buildBaseURL("127.0.0.1", 0), "http://127.0.0.1:8899");
eq("remote host refused", buildBaseURL("evil.example", 8899), "http://127.0.0.1:8899");
eq("ipv6 bracketed", buildBaseURL("::1", 9123), "http://[::1]:9123");

console.log("\n-- parseBaseURL --");
eq("round trip", parseBaseURL("http://127.0.0.1:9123"), { host: "127.0.0.1", port: 9123 });
eq("no port", parseBaseURL("http://127.0.0.1"), { host: "127.0.0.1", port: 8899 });
eq("garbage", parseBaseURL("not a url"), { host: "127.0.0.1", port: 8899 });
eq("empty", parseBaseURL(""), { host: "127.0.0.1", port: 8899 });
eq("javascript scheme", parseBaseURL("javascript:alert(1)"), { host: "127.0.0.1", port: 8899 });
eq("remote host", parseBaseURL("http://10.0.0.9:8899"), { host: "127.0.0.1", port: 8899 });

console.log("\n-- originPattern / round trip through build --");
const saved = buildBaseURL("localhost", 9123);
eq("saved", saved, "http://localhost:9123");
eq("parsed back", parseBaseURL(saved), { host: "localhost", port: 9123 });
eq("pattern", originPattern(saved), "http://localhost:9123/*");
eq("pattern default", originPattern(buildBaseURL(DEFAULT_HOST, DEFAULT_PORT)), "http://127.0.0.1:8899/*");

console.log(failures === 0 ? "\nall config checks passed" : `\n${failures} config check(s) failed`);
process.exit(failures === 0 ? 0 : 1);
