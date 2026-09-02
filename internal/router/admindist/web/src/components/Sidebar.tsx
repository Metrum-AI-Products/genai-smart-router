// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

import { BarChart3, Boxes, Gauge, LineChart, LockKeyhole, Network, SearchCheck, ShieldCheck, SlidersHorizontal, WalletCards } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type React from "react";
import { SidebarGroup } from "@/components/SidebarGroup";
import { groupedTabs, navGroups, type NavGroupId } from "@/lib/navGroups";
import type { TabSpec } from "@/lib/reports";
import { cn } from "@/lib/utils";

const collapsedStorageKey = "admin-nav-collapsed-v1";

const groupIcons: Record<NavGroupId, React.ComponentType<{ className?: string; "aria-hidden"?: boolean }>> = {
  overview: Gauge,
  usage: BarChart3,
  savings: WalletCards,
  performance: LineChart,
  "traffic-shaping": SlidersHorizontal,
  "routing-decisions": Network,
  "provider-catalog": Boxes,
  security: ShieldCheck,
  "request-drilldown": SearchCheck,
  "system-status": LockKeyhole,
  operations: LockKeyhole,
};

type SidebarProps = {
  tabs: TabSpec[];
  activeTab: string;
  onTabChange: (tabId: string) => void;
  showDescriptions?: boolean;
  className?: string;
};

function readCollapsedGroups() {
  try {
    const raw = window.localStorage.getItem(collapsedStorageKey);
    if (!raw) return new Set<string>();
    const parsed = JSON.parse(raw);
    return new Set(Array.isArray(parsed) ? parsed.filter((id): id is string => typeof id === "string") : []);
  } catch {
    return new Set<string>();
  }
}

export function Sidebar({ tabs, activeTab, onTabChange, showDescriptions = false, className }: SidebarProps) {
  const groups = useMemo(() => groupedTabs(navGroups, tabs), [tabs]);
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(() => readCollapsedGroups());

  useEffect(() => {
    try {
      window.localStorage.setItem(collapsedStorageKey, JSON.stringify(Array.from(collapsedGroups).sort()));
    } catch {
      // Storage can be disabled in hardened admin browsers. Keep navigation usable.
    }
  }, [collapsedGroups]);

  function toggleGroup(groupId: string) {
    setCollapsedGroups((current) => {
      const next = new Set(current);
      if (next.has(groupId)) next.delete(groupId);
      else next.add(groupId);
      return next;
    });
  }

  return (
    <nav
      className={cn("rounded-lg border border-white/10 bg-white/[0.035] p-2", className)}
      aria-label="Report sections"
      data-report-sidebar
    >
      <div className="space-y-1">
        {groups.map((group) => {
          const Icon = groupIcons[group.id];
          return (
            <div key={group.id} className="grid grid-cols-[1.25rem_1fr] gap-2">
              <div className="flex justify-center pt-2 text-white/42">
                <Icon className="h-4 w-4" aria-hidden={true} />
              </div>
              <SidebarGroup
                id={group.id}
                label={group.label}
                description={group.description}
                tabIds={group.tabIds}
                tabs={group.tabs}
                activeTab={activeTab}
                collapsed={collapsedGroups.has(group.id)}
                showDescriptions={showDescriptions}
                onToggle={toggleGroup}
                onSelectTab={onTabChange}
              />
            </div>
          );
        })}
      </div>
    </nav>
  );
}
