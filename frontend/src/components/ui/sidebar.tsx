import * as React from "react";
import { cn } from "../../lib/utils";

const Sidebar = React.forwardRef<HTMLElement, React.HTMLAttributes<HTMLElement>>(
  ({ className, ...props }, ref) => (
    <aside
      ref={ref}
      data-slot="sidebar"
      className={cn(
        "flex h-full w-[200px] min-h-0 shrink-0 flex-col overflow-y-auto border-r border-[var(--border)] bg-[var(--panel)] px-2.5 py-3.5",
        className,
      )}
      {...props}
    />
  ),
);
Sidebar.displayName = "Sidebar";

const SidebarNav = React.forwardRef<HTMLElement, React.HTMLAttributes<HTMLElement>>(
  ({ className, ...props }, ref) => (
    <nav ref={ref} data-slot="sidebar-nav" className={cn("flex flex-col gap-0.5", className)} {...props} />
  ),
);
SidebarNav.displayName = "SidebarNav";

const SidebarSection = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      data-slot="sidebar-section"
      className={cn(
        "mt-3.5 flex items-center justify-between px-1 text-xs uppercase tracking-[0.4px] text-[var(--muted)]",
        className,
      )}
      {...props}
    />
  ),
);
SidebarSection.displayName = "SidebarSection";

export { Sidebar, SidebarNav, SidebarSection };
