import type { TabSpec } from "@/lib/reports";
import { cn } from "@/lib/utils";

type SidebarItemProps = {
  tab: TabSpec;
  active: boolean;
  onSelect: (tabId: string) => void;
};

export function SidebarItem({ tab, active, onSelect }: SidebarItemProps) {
  return (
    <button
      type="button"
      data-report-tab={tab.id}
      aria-current={active ? "page" : undefined}
      className={cn(
        "flex w-full items-center rounded-md px-3 py-2 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-metrum-blue",
        active ? "bg-metrum-purple text-white" : "text-white/64 hover:bg-white/[0.07] hover:text-white",
      )}
      onClick={() => onSelect(tab.id)}
    >
      <span className="truncate">{tab.label}</span>
    </button>
  );
}
