import type { Ref } from "react";
import { IconButton } from "../ui/astryx";
import { Input } from "../ui/input";
import { Icon } from "../ui/icon";

export default function SearchBar({
  value,
  onChange,
  placeholder = "搜索视频",
  inputRef,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  // Lets the owner of the bar focus it from a keyboard shortcut.
  inputRef?: Ref<HTMLInputElement>;
}) {
  return (
    <div className="relative flex w-[280px] items-center" data-tour="search">
      <Icon className="pointer-events-none absolute left-3 z-10 size-4 text-[var(--muted)]" name="search" />
      <Input
        ref={inputRef}
        className="pl-9 pr-9"
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        spellCheck={false}
      />
      {value && (
        <IconButton
          className="absolute right-1 h-7 w-7 p-0"
          icon={<Icon className="size-4" name="x" />}
          label="清除搜索"
          onClick={() => onChange("")}
          size="sm"
          tooltip="清除"
          variant="ghost"
        />
      )}
    </div>
  );
}
