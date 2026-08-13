import { cn } from "~/lib/utils";

interface ProgressProps {
  value?: number;
  indeterminate?: boolean;
  className?: string;
}

export function Progress({ value, indeterminate, className }: ProgressProps) {
  return (
    <div
      className={cn(
        "h-2 w-full overflow-hidden rounded-full bg-[var(--accent)]",
        className,
      )}
      role="progressbar"
      aria-valuenow={indeterminate ? undefined : value}
      aria-valuemin={0}
      aria-valuemax={100}
    >
      {indeterminate ? (
        <div className="h-full w-1/3 animate-[progress-indeterminate_1.4s_ease-in-out_infinite] rounded-full bg-[var(--lagoon)]" />
      ) : (
        <div
          className="h-full rounded-full bg-[var(--lagoon)] transition-[width] duration-300 ease-out"
          style={{ width: `${Math.min(100, Math.max(0, value ?? 0))}%` }}
        />
      )}
    </div>
  );
}
