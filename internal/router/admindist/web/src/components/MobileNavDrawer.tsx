// Copyright 2026 Metrum AI, Inc.
// SPDX-License-Identifier: Apache-2.0

import { X } from "lucide-react";
import { Sidebar } from "@/components/Sidebar";
import { Button } from "@/components/ui/button";
import type { TabSpec } from "@/lib/reports";

type MobileNavDrawerProps = {
  open: boolean;
  tabs: TabSpec[];
  activeTab: string;
  onOpenChange: (open: boolean) => void;
  onTabChange: (tabId: string) => void;
};

export function MobileNavDrawer({ open, tabs, activeTab, onOpenChange, onTabChange }: MobileNavDrawerProps) {
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 lg:hidden" role="dialog" aria-modal="true" aria-label="Report navigation">
      <button
        type="button"
        className="absolute inset-0 bg-black/70"
        aria-label="Close report navigation"
        onClick={() => onOpenChange(false)}
      />
      <div className="relative flex h-full w-[min(22rem,calc(100vw-2rem))] flex-col border-r border-white/10 bg-black p-3 shadow-2xl">
        <div className="mb-3 flex items-center justify-between">
          <div>
            <p className="font-mono text-[0.68rem] uppercase text-white/50">Admin reports</p>
            <p className="font-display text-xl text-white">Sections</p>
          </div>
          <Button type="button" variant="ghost" className="h-9 w-9 px-0" aria-label="Close report navigation" onClick={() => onOpenChange(false)}>
            <X className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
        <Sidebar
          tabs={tabs}
          activeTab={activeTab}
          showDescriptions={true}
          className="min-h-0 flex-1 overflow-y-auto"
          onTabChange={(tabId) => {
            onTabChange(tabId);
            onOpenChange(false);
          }}
        />
      </div>
    </div>
  );
}
