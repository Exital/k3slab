export function MIcon({ name, className = "", filled }: { name: string; className?: string; filled?: boolean }) {
  return (
    <span className={`material-symbols-outlined ${filled ? "filled" : ""} ${className}`.trim()} aria-hidden>
      {name}
    </span>
  );
}
