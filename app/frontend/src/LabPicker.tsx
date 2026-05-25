import { MIcon } from "./components/MIcon";
import type { LabCatalog, LabEntry } from "./api";

type LabPickerProps = {
  catalog: LabCatalog;
  highlightId?: string;
  busy?: boolean;
  onSelect: (id: string) => void;
};

function LabCard({
  lab,
  highlighted,
  disabled,
  onSelect,
}: {
  lab: LabEntry;
  highlighted: boolean;
  disabled: boolean;
  onSelect: () => void;
}) {
  const title = lab.name || lab.id;
  return (
    <button
      type="button"
      disabled={disabled}
      title={lab.error}
      onClick={onSelect}
      className={
        "flex w-full flex-col gap-2 rounded-xl border p-4 text-left transition " +
        (disabled
          ? "cursor-not-allowed border-slate-300/50 bg-slate-100/60 opacity-60 dark:border-k3-outline-variant/40 dark:bg-k3-surface-container/40"
          : highlighted
            ? "border-blue-600 bg-blue-50/80 shadow-sm hover:border-blue-700 dark:border-k3-primary dark:bg-k3-primary-container/20 dark:hover:border-k3-primary"
            : "border-slate-400/35 bg-white/90 hover:border-teal-600/50 hover:bg-white dark:border-k3-outline-variant dark:bg-k3-surface-container dark:hover:border-k3-secondary/50")
      }
    >
      <div className="flex items-start justify-between gap-2">
        <span className="font-display text-lg font-bold text-slate-900 dark:text-k3-on-surface">{title}</span>
        <span className="shrink-0 font-mono text-xs text-slate-500 dark:text-k3-on-surface-variant">{lab.id}</span>
      </div>
      {lab.description ? (
        <p className="text-sm text-slate-600 dark:text-k3-on-surface-variant">{lab.description}</p>
      ) : null}
      {lab.valid && lab.stepCount != null ? (
        <p className="font-mono text-xs text-slate-500 dark:text-k3-on-surface-variant">{lab.stepCount} steps</p>
      ) : null}
      {!lab.valid && lab.error ? (
        <p className="text-xs text-rose-700 dark:text-k3-error">{lab.error}</p>
      ) : null}
    </button>
  );
}

export function LabPicker({ catalog, highlightId, busy, onSelect }: LabPickerProps) {
  const valid = catalog.labs.filter((l) => l.valid);
  return (
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center overflow-auto bg-[#d4d9e3]/50 p-6 dark:bg-k3-surface-low">
      <div className="w-full max-w-lg">
        <div className="mb-6 text-center">
          <MIcon name="science" className="!text-4xl text-blue-800 dark:text-k3-primary" />
          <h1 className="mt-2 font-display text-2xl font-extrabold text-slate-900 dark:text-k3-primary">Choose a lab</h1>
          <p className="mt-1 text-sm text-slate-600 dark:text-k3-on-surface-variant">
            Switching labs resets the cluster. Pick a workshop to begin.
          </p>
        </div>
        <div className="flex flex-col gap-3">
          {catalog.labs.map((lab) => (
            <LabCard
              key={lab.id}
              lab={lab}
              highlighted={lab.id === highlightId}
              disabled={!lab.valid || !!busy}
              onSelect={() => onSelect(lab.id)}
            />
          ))}
        </div>
        {valid.length === 0 ? (
          <p className="mt-4 text-center text-sm text-rose-700 dark:text-k3-error">
            No valid labs found under {catalog.labsRoot}. Each subdirectory needs a workshop.yml.
          </p>
        ) : null}
      </div>
    </div>
  );
}

type LabSwitcherProps = {
  catalog: LabCatalog;
  activeId?: string;
  open: boolean;
  busy?: boolean;
  onToggle: () => void;
  onClose: () => void;
  onSelect: (id: string) => void;
};

export function LabSwitcher({ catalog, activeId, open, busy, onToggle, onClose, onSelect }: LabSwitcherProps) {
  const valid = catalog.labs.filter((l) => l.valid);
  if (valid.length <= 1) return null;

  return (
    <div className="relative hidden md:block">
      <button
        type="button"
        className="flex items-center gap-1 border-b-2 border-blue-700 pb-1 font-mono text-xs font-semibold uppercase tracking-wider text-blue-900 dark:border-k3-primary dark:font-medium dark:text-k3-primary"
        aria-expanded={open}
        aria-haspopup="listbox"
        disabled={busy}
        onClick={onToggle}
      >
        Labs
        <MIcon name={open ? "expand_less" : "expand_more"} className="!text-base" />
      </button>
      {open ? (
        <>
          <div className="fixed inset-0 z-40" aria-hidden onClick={onClose} />
          <ul
            role="listbox"
            className="absolute left-0 top-full z-50 mt-2 min-w-[16rem] rounded-lg border border-slate-400/30 bg-white py-1 shadow-lg dark:border-k3-outline-variant dark:bg-k3-surface-container"
          >
            {catalog.labs.map((lab) => {
              const disabled = !lab.valid || busy;
              const selected = lab.id === activeId;
              return (
                <li key={lab.id} role="option" aria-selected={selected}>
                  <button
                    type="button"
                    disabled={disabled}
                    title={lab.error}
                    className={
                      "flex w-full flex-col gap-0.5 px-3 py-2 text-left text-sm " +
                      (disabled
                        ? "cursor-not-allowed opacity-50"
                        : selected
                          ? "bg-blue-50 text-blue-950 dark:bg-k3-primary-container/25 dark:text-k3-primary"
                          : "text-slate-800 hover:bg-slate-100 dark:text-k3-on-surface dark:hover:bg-k3-surface-container-high")
                    }
                    onClick={() => {
                      onClose();
                      if (!selected) onSelect(lab.id);
                    }}
                  >
                    <span className="font-medium">{lab.name || lab.id}</span>
                    {lab.description ? (
                      <span className="text-xs text-slate-500 dark:text-k3-on-surface-variant line-clamp-2">
                        {lab.description}
                      </span>
                    ) : null}
                  </button>
                </li>
              );
            })}
          </ul>
        </>
      ) : null}
    </div>
  );
}
