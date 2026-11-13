import type { Ref } from "react";
import { Search, X } from "lucide-react";
import { Button } from "../ui/button";
import { Input } from "../ui/input";

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
      <Search className="pointer-events-none absolute left-3 h-4 w-4 text-[var(--muted)]" />
      <Input
        ref={inputRef}
        className="pl-9 pr-9"
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        spellCheck={false}
      />
      {value && (
        <Button
          variant="ghost"
          size="sm"
          className="absolute right-1 h-7 w-7 p-0"
          title="清除"
          aria-label="清除搜索"
          onClick={() => onChange("")}
        >
          <X className="h-4 w-4" />
        </Button>
      )}
    </div>
  );
}
