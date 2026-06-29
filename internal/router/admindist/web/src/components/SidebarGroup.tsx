import { ChevronDown } from "lucide-react";
import { SidebarItem } from "@/components/SidebarItem";
import type { NavGroup } from "@/lib/navGroups";
import type { TabSpec } from "@/lib/reports";
import { cn } from "@/lib/utils";

type SidebarGroupProps = NavGroup & {
  tabs: TabSpec[];
  activeTab: string;
  collapsed: boolean;
  onToggle: (groupId: string) => void;
  onSelectTab: (tabId: string) => void;
};

export function SidebarGroup({ id, label, tabs, activeTab, collapsed, onToggle, onSelectTab }: SidebarGroupProps) {
  const listId = `admin-report-nav-${id}`;
  const containsActiveTab = tabs.some((tab) => tab.id === activeTab);

  return (
    <section className="space-y-1" data-nav-group={id}>
      <button
        type="button"
        className={cn(
          "flex w-full items-center justify-between rounded-md px-2 py-1.5 font-mono text-[0.68rem] uppercase text-white/50 transition-colors hover:bg-white/[0.05] hover:text-white/74 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-metrum-blue",
          containsActiveTab && "text-white/74",
        )}
        aria-expanded={!collapsed}
        aria-controls={listId}
        onClick={() => onToggle(id)}
      >
        <span>{label}</span>
        <ChevronDown className={cn("h-3.5 w-3.5 transition-transform motion-reduce:transition-none", collapsed && "-rotate-90")} aria-hidden="true" />
      </button>
      <div id={listId} className="space-y-1" hidden={collapsed}>
        {tabs.map((tab) => (
          <SidebarItem key={tab.id} tab={tab} active={tab.id === activeTab} onSelect={onSelectTab} />
        ))}
      </div>
    </section>
  );
}
