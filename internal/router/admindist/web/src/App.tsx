import { useEffect, useMemo, useRef, useState } from "react";
import { Menu } from "lucide-react";
import { MobileNavDrawer } from "@/components/MobileNavDrawer";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ReportPanel } from "@/components/ReportPanel";
import { Sidebar } from "@/components/Sidebar";
import { fetchReport, fetchVersion, filterFields, tabSpecs, type ReportFilters, type ReportResponse, type TabSpec, type VersionResponse } from "@/lib/reports";

function filtersFromUrl(): ReportFilters {
  const params = new URLSearchParams(window.location.search);
  const filters: ReportFilters = {};
  for (const [name, , defaultValue] of filterFields) {
    filters[name] = params.get(name) || defaultValue;
  }
  filters.limit = params.get("limit") || "50";
  return filters;
}

function activeTabFromUrl() {
  const tab = new URLSearchParams(window.location.search).get("tab");
  return tabSpecs.some((spec) => spec.id === tab) ? tab || "groups" : "groups";
}

export default function App() {
  const [activeTab, setActiveTab] = useState(activeTabFromUrl);
  const [filters, setFilters] = useState<ReportFilters>(filtersFromUrl);
  const [draftFilters, setDraftFilters] = useState<ReportFilters>(filtersFromUrl);
  const [report, setReport] = useState<ReportResponse>();
  const [version, setVersion] = useState<VersionResponse>();
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const loadSequence = useRef(0);

  const tab = useMemo(() => tabSpecs.find((spec) => spec.id === activeTab) || tabSpecs[1], [activeTab]);

  async function load(nextTab: TabSpec = tab, nextFilters: ReportFilters = filters) {
    const sequence = ++loadSequence.current;
    setLoading(true);
    setError("");
    try {
      const endpoint = nextTab.endpoint === "overview" ? "summary" : nextTab.endpoint;
      const nextReport = await fetchReport(endpoint, nextFilters);
      if (sequence !== loadSequence.current) return;
      setReport(nextReport);
    } catch (err) {
      if (sequence !== loadSequence.current) return;
      setError(err instanceof Error ? err.message : "Report request failed");
      setReport(undefined);
    } finally {
      if (sequence === loadSequence.current) setLoading(false);
    }
  }

  useEffect(() => {
    void fetchVersion()
      .then(setVersion)
      .catch(() => setVersion(undefined));
  }, []);

  useEffect(() => {
    const url = new URL(window.location.href);
    url.searchParams.set("tab", activeTab);
    for (const [key, value] of Object.entries(filters)) {
      if (value.trim()) url.searchParams.set(key, value.trim());
      else url.searchParams.delete(key);
    }
    history.replaceState(null, "", url);
    void load(tab, filters);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTab, filters]);

  function applyFilters(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setFilters(draftFilters);
  }

  const versionLabel = version?.version ? `v${version.version}` : "";
  const commitLabel = version?.commit && version.commit !== "unknown" ? version.commit.slice(0, 12) : "";
  const buildLabel = version?.build_date && version.build_date !== "unknown" ? version.build_date : "";

  return (
    <div className="mx-auto w-full max-w-[1700px] space-y-5 px-4 py-5 lg:px-6">
      <header className="rounded-lg border border-white/10 bg-black p-4 shadow-2xl">
        <div className="grid gap-4 xl:grid-cols-[360px_1fr]">
          <div className="flex min-w-0 items-center gap-4">
            <img className="w-44 max-w-[42vw]" src="static/metrum_logo_white_new.png" alt="Metrum AI" />
            <div className="min-w-0">
              <p className="font-mono text-xs uppercase text-white/58">GenAI Smart Router</p>
              <h1 className="font-display text-3xl text-white">Admin Reports</h1>
              <p className="text-sm text-white/58">Operational usage, savings, routing, and security reporting.</p>
              {version && (
                <div className="mt-2 flex flex-wrap gap-2 font-mono text-[0.68rem] uppercase text-white/66">
                  {versionLabel && <span className="rounded border border-white/14 bg-white/[0.06] px-2 py-1 text-white">{versionLabel}</span>}
                  {commitLabel && <span className="rounded border border-white/14 bg-white/[0.035] px-2 py-1">Commit {commitLabel}</span>}
                  {buildLabel && <span className="rounded border border-white/14 bg-white/[0.035] px-2 py-1">Built {buildLabel}</span>}
                  {version.license_compile_mode && <span className="rounded border border-metrum-purple/45 bg-metrum-purple/15 px-2 py-1 text-white/80">{version.license_compile_mode}</span>}
                </div>
              )}
            </div>
          </div>
          <form className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6" onSubmit={applyFilters}>
            {filterFields.map(([name, label, defaultValue]) => (
              <label key={name} className="grid gap-1 font-mono text-[0.68rem] uppercase text-white/58">
                {label}
                <Input
                  name={name}
                  value={draftFilters[name] ?? defaultValue}
                  placeholder={defaultValue || "any"}
                  onChange={(event) => setDraftFilters((current) => ({ ...current, [name]: event.target.value }))}
                />
              </label>
            ))}
            <label className="grid gap-1 font-mono text-[0.68rem] uppercase text-white/58">
              Rows
              <Input
                name="limit"
                value={draftFilters.limit ?? "50"}
                inputMode="numeric"
                onChange={(event) => setDraftFilters((current) => ({ ...current, limit: event.target.value }))}
              />
            </label>
            <div className="flex items-end gap-2">
              <Button type="submit">Apply</Button>
              <Button type="button" variant="outline" className="lg:hidden" aria-label="Open report navigation" onClick={() => setMobileNavOpen(true)}>
                <Menu className="mr-2 h-4 w-4" aria-hidden="true" />
                Sections
              </Button>
              <a className="inline-flex h-9 items-center rounded-md border border-white/15 px-3 text-sm text-white hover:bg-white/[0.08]" href={`export.md?${new URLSearchParams(filters)}`}>
                Markdown
              </a>
            </div>
          </form>
        </div>
      </header>
      <div className="grid min-w-0 gap-5 lg:grid-cols-[17rem_minmax(0,1fr)]">
        <Sidebar tabs={tabSpecs} activeTab={activeTab} onTabChange={setActiveTab} className="sticky top-5 hidden max-h-[calc(100vh-2.5rem)] overflow-y-auto lg:block" />
        <div className="min-w-0">
          <ReportPanel tab={tab} report={report} filters={filters} loading={loading} error={error} onRefresh={() => void load(tab, filters)} />
        </div>
      </div>
      <MobileNavDrawer open={mobileNavOpen} tabs={tabSpecs} activeTab={activeTab} onOpenChange={setMobileNavOpen} onTabChange={setActiveTab} />
    </div>
  );
}
